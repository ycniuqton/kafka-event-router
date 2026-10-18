import { describe, expect, it } from 'vitest';
import { BulkAction, DeliveryMode, ReplayStatus, RouteDraftFactory, RouteStatus, ValidationMode } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpRoutesService } from './HttpRoutesService';
import type { RouteDetailDto } from './RouteDtos';

function detail(over: Partial<RouteDetailDto> = {}): RouteDetailDto {
  return {
    id: 'r1',
    name: 'orders',
    teamId: 't1',
    teamName: 'payments',
    tags: [],
    status: RouteStatus.Running,
    activeVersion: { id: 'v1', no: 1 },
    hasUndeployedDraft: false,
    source: {
      connectionId: 'c1',
      connectionName: 'orders-prod',
      topics: ['order-events'],
    },
    filterSummary: { headline: 'eventType is OrderPaid', more: 1 },
    transform: 'js',
    destinations: [
      {
        branchId: 'main',
        connectionId: 'c2',
        connectionName: 'partner',
        topic: 'out',
      },
    ],
    metrics: null,
    updatedAt: '2026-10-13T09:00:00Z',
    updatedBy: null,
    description: '',
    replicas: 2,
    revision: 4,
    draft: {
      apiVersion: 'router/v1',
      name: 'orders',
      source: {
        connectionId: 'c1',
        topics: ['order-events'],
        format: { type: 'json' },
      },
      branches: [{ id: 'main', destination: { connectionId: 'c2', topic: 'out' } }],
    },
    active: null,
    lastDeployment: null,
    ...over,
  };
}

const diff = {
  a: '1',
  b: '2',
  sections: [
    {
      section: 'filter',
      lines: [{ mark: '~', text: 'amount > 100 → amount > 200', path: '/x' }],
    },
  ],
};

function setup() {
  const fetch = new TestFetch();
  return {
    fetch,
    service: new HttpRoutesService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })),
  };
}

describe('HttpRoutesService', () => {
  it('lists routes and maps summaries', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [detail()], next_cursor: null });
    const [route] = await service.list();
    expect(fetch.calls[0].url).toBe('/api/v1/routes?limit=200');
    expect(route).toMatchObject({
      id: 'r1',
      owner: 'payments',
      filterSummary: 'eventType is OrderPaid +1',
      version: 1,
      destination: { topic: 'out' },
    });
  });

  it('opens a draft, then saves it with the ETag it was read with', async () => {
    const { fetch, service } = setup();
    fetch.json(200, detail(), { ETag: '"4"' }).json(200, detail({ revision: 5, issues: [] }), { ETag: '"5"' });
    const draft = await service.openDraft('r1');
    draft.name = 'orders-v2';
    await service.saveDraft(draft);
    expect(fetch.calls[1].init.method).toBe('PATCH');
    expect(fetch.header(1, 'If-Match')).toBe('"4"');
    const body = JSON.parse(String(fetch.calls[1].init.body));
    expect(body.name).toBe('orders-v2');
    expect(body.draft.source.connectionId).toBe('c1');
  });

  it('creates the route on the first save and deploys with the fresh ETag', async () => {
    const { fetch, service } = setup();
    fetch
      .json(201, detail({ status: RouteStatus.Draft, revision: 1 }), {
        ETag: '"1"',
      })
      .json(
        202,
        {
          deployment: {
            id: 'd1',
            versionNo: 2,
            status: 'rolling_out',
            requestedAt: '',
          },
          version: {
            id: 'v2',
            no: 2,
            origin: 'deploy',
            author: { id: 'u', name: 'Quincy' },
            changeNote: 'go',
            createdAt: 'now',
          },
          route: detail({ revision: 3 }),
        },
        { ETag: '"3"' },
      )
      .json(200, diff);
    const result = await service.deploy(RouteDraftFactory.create(null, null), 'go');
    expect(fetch.calls[0].init.method).toBe('POST');
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/deploy');
    expect(fetch.header(1, 'If-Match')).toBe('"1"');
    expect(fetch.calls[2].url).toBe('/api/v1/routes/r1/versions/1/diff/2');
    expect(result.version).toMatchObject({
      version: 2,
      author: 'Quincy',
      note: 'go',
      diff: [{ mark: '~', text: 'amount > 100 → amount > 200' }],
    });
    expect(result.activity.detail).toBe('v2');
  });

  it('starts a new draft on the first real connection instead of mock ids', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      items: [{ id: 'c-real', name: 'staging-kafka' }],
      next_cursor: null,
    });
    const draft = await service.openDraft(null);
    expect(fetch.calls[0].url).toContain('/connections?limit=1');
    expect(draft.source.connectionId).toBe('c-real');
    expect(draft.destination.connectionId).toBe('c-real');
  });

  it('surfaces 412 on stale saves and 404 as undefined on get', async () => {
    const { fetch, service } = setup();
    fetch.json(200, detail(), { ETag: '"4"' }).problem(412, {
      type: 'https://eventrouter.dev/problems/precondition-failed',
      title: 'Precondition Failed',
      status: 412,
      current_etag: '"6"',
    });
    const draft = await service.openDraft('r1');
    const error = await TestFetch.failure(service.saveDraft(draft));
    expect(error.currentEtag).toBe('"6"');
    fetch.problem(404, {
      type: 'https://eventrouter.dev/problems/not-found',
      title: 'Not Found',
      status: 404,
    });
    expect(await service.get('nope')).toBeUndefined();
  });

  it('validates, diffs, lists deployments and runs bulk actions', async () => {
    const { fetch, service } = setup();
    fetch
      .json(200, {
        valid: false,
        issues: [
          {
            code: 'topic_missing',
            severity: 'error',
            path: '/branches/0/destination/topic',
            message: 'Missing.',
          },
        ],
      })
      .json(200, diff)
      .json(200, {
        items: [{ id: 'd1', versionNo: 1, status: 'active', requestedAt: 'x' }],
        next_cursor: null,
      })
      .json(200, {
        results: [
          { id: 'r1', ok: true },
          {
            id: 'r2',
            ok: false,
            problem: {
              title: 'Not Found',
              detail: 'This route does not exist.',
            },
          },
        ],
      });
    const result = await service.validate('r1', null, ValidationMode.Deploy);
    expect(result.valid).toBe(false);
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({
      mode: 'deploy',
    });
    expect((await service.diff('r1', 'active', 'draft')).sections).toHaveLength(1);
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/versions/active/diff/draft');
    expect((await service.listDeployments('r1'))[0]).toMatchObject({
      versionNo: 1,
      failureReason: '',
      finishedAt: null,
    });
    const bulk = await service.bulk(BulkAction.AddTag, ['r1', 'r2'], 'pii');
    expect(bulk).toEqual([
      { id: 'r1', ok: true, message: null },
      { id: 'r2', ok: false, message: 'This route does not exist.' },
    ]);
  });

  it('maps connection names, tags and missing metrics on summaries', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [detail({ tags: ['pii'] })], next_cursor: null });
    const [route] = await service.list();
    expect(route.source.connectionName).toBe('orders-prod');
    expect(route.destination.connectionName).toBe('partner');
    expect(route.tags).toEqual(['pii']);
    expect(route.metrics.unavailable).toBe(true);
  });

  it('reads settings and saves error policy and limits into the draft with the loaded ETag', async () => {
    const { fetch, service } = setup();
    const draftDoc = {
      ...detail().draft!,
      errorPolicy: { onError: 'skip' as const, maxRetries: 2 },
      limits: { maxInFlight: 10 },
    };
    fetch.json(200, detail({ draft: draftDoc, tags: ['a'] }), { ETag: '"4"' }).json(200, detail({ revision: 5, replicas: 3 }), { ETag: '"5"' });
    const settings = await service.settings('r1');
    expect(settings).toMatchObject({
      replicas: 2,
      tags: ['a'],
      errorPolicy: { onError: 'skip', maxRetries: 2, dlqTopic: '' },
      limits: { maxInFlight: 10, jsTimeoutMs: null },
    });
    await service.updateSettings('r1', {
      replicas: 3,
      errorPolicy: {
        onError: 'dlq',
        dlqConnectionId: '',
        dlqTopic: 'my.dlq',
        maxRetries: null,
      },
      limits: { ...settings.limits, maxInFlight: null, jsTimeoutMs: 500 },
    });
    expect(fetch.calls[1].init.method).toBe('PATCH');
    expect(new Headers(fetch.calls[1].init.headers).get('If-Match')).toBe('"4"');
    const body = JSON.parse(String(fetch.calls[1].init.body));
    expect(body.replicas).toBe(3);
    expect(body.draft.errorPolicy).toEqual({
      onError: 'dlq',
      dlqTopic: 'my.dlq',
    });
    expect(body.draft.limits).toEqual({ jsTimeoutMs: 500 });
  });

  it('reads the delivery mode and writes delivery only when it changes', async () => {
    const { fetch, service } = setup();
    fetch.json(200, detail(), { ETag: '"4"' }).json(200, detail({ revision: 5 }), { ETag: '"5"' }).json(200, detail({ revision: 6 }), { ETag: '"6"' });
    const settings = await service.settings('r1');
    expect(settings.deliveryMode).toBe(DeliveryMode.AtLeastOnce);
    await service.updateSettings('r1', { deliveryMode: DeliveryMode.AtLeastOnce });
    expect(JSON.parse(String(fetch.calls[1].init.body)).draft.delivery).toBeUndefined();
    await service.updateSettings('r1', { deliveryMode: DeliveryMode.ExactlyOnce });
    expect(JSON.parse(String(fetch.calls[2].init.body)).draft.delivery).toEqual({ mode: 'exactly_once' });
  });

  it('requests a replay and reads its status', async () => {
    const { fetch, service } = setup();
    const dto = { replayId: 'rp1', routeId: 'r1', status: 'pending', from: '2026-10-14T08:00:00Z', requestedAt: '2026-10-14T09:00:00Z', partitions: 0 };
    fetch.json(202, dto).json(200, { ...dto, status: 'done', startedAt: '2026-10-14T09:00:01Z', finishedAt: '2026-10-14T09:00:03Z', partitions: 3 });
    const replay = await service.replay('r1', { from: '2026-10-14T08:00:00Z', confirm: true });
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r1/replay');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ from: '2026-10-14T08:00:00Z', confirm: true });
    expect(replay).toMatchObject({ id: 'rp1', status: ReplayStatus.Pending, startedAt: null, finishedAt: null, error: '' });
    const done = await service.replayStatus('r1', 'rp1');
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/replays/rp1');
    expect(done).toMatchObject({ status: ReplayStatus.Done, partitions: 3 });
  });

  it('surfaces a missing replay confirmation as a field error', async () => {
    const { fetch, service } = setup();
    fetch.json(422, { type: 'about:blank', title: 'Validation failed', status: 422, errors: [{ path: '/confirm', code: 'confirm_required', message: 'Confirm.' }] }, { 'Content-Type': 'application/problem+json' });
    await expect(service.replay('r1', { from: '2026-10-14T08:00:00Z', confirm: false })).rejects.toMatchObject({ status: 422 });
  });
});
