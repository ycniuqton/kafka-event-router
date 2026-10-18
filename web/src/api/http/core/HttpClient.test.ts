import { describe, expect, it } from 'vitest';
import { ApiErrorKind, type ApiError } from './ApiError';
import { ApiErrorListener } from './ApiErrorBus';
import { HttpContext } from './HttpContext';
import { HttpMethod } from './HttpConstants';
import { ProblemKind } from './ProblemKind';
import { TestFetch } from './TestFetch.test-util';

class Recorder extends ApiErrorListener {
  readonly seen: ApiError[] = [];
  handle(error: ApiError): void {
    this.seen.push(error);
  }
}

function setup() {
  const fetch = new TestFetch();
  const context = new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn });
  const recorder = new Recorder();
  context.errors.add(recorder);
  return { fetch, context, recorder };
}

describe('HttpClient', () => {
  it('sends JSON with cookies, CSRF on writes and If-Match, and returns the ETag', async () => {
    const { fetch, context } = setup();
    context.csrf.set('csrf-1');
    fetch.json(200, { id: 'r1' }, { ETag: '"7"', 'X-Request-ID': 'req-1' });
    const res = await context.api.send<{ id: string }>({ method: HttpMethod.Patch, path: '/routes/r1', body: { name: 'x' }, ifMatch: '"6"' });
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r1');
    expect(fetch.calls[0].init.credentials).toBe('include');
    expect(fetch.calls[0].init.body).toBe('{"name":"x"}');
    expect(fetch.header(0, 'X-CSRF-Token')).toBe('csrf-1');
    expect(fetch.header(0, 'If-Match')).toBe('"6"');
    expect(fetch.header(0, 'Content-Type')).toBe('application/json');
    expect(res).toMatchObject({ data: { id: 'r1' }, etag: '"7"', requestId: 'req-1' });
  });

  it('does not send the CSRF token on GET and drops empty query values', async () => {
    const { fetch, context } = setup();
    context.csrf.set('csrf-1');
    fetch.json(200, []);
    await context.api.get('/routes', { q: 'orders', cursor: null, limit: 50 });
    expect(fetch.calls[0].url).toBe('/api/v1/routes?q=orders&limit=50');
    expect(fetch.header(0, 'X-CSRF-Token')).toBeNull();
  });

  it('resolves 204 to undefined', async () => {
    const { fetch, context } = setup();
    fetch.empty(204);
    await expect(context.auth.post('/auth/logout')).resolves.toBeUndefined();
    expect(fetch.calls[0].url).toBe('/auth/logout');
  });

  it('throws an ApiError from a problem body and publishes it', async () => {
    const { fetch, context, recorder } = setup();
    fetch.problem(422, {
      type: 'https://eventrouter.dev/problems/validation',
      title: 'Unprocessable Entity',
      status: 422,
      detail: '2 fields need attention.',
      path: '/bootstrapServers/0',
      request_id: '01J',
      errors: [
        { path: '/bootstrapServers/0', code: 'host_port', message: 'Use host:port.' },
        { path: '/name', code: 'duplicate', message: 'Taken.' },
      ],
    });
    const error = await TestFetch.failure(context.api.post('/connections', {}));
    expect(error.kind).toBe(ApiErrorKind.Problem);
    expect(error.problemKind).toBe(ProblemKind.Validation);
    expect(error.requestId).toBe('01J');
    expect(error.fieldErrors()).toEqual({ '/bootstrapServers/0': 'Use host:port.', '/name': 'Taken.' });
    expect(recorder.seen).toEqual([error]);
  });

  it('synthesises a problem for non-problem error bodies, using the status', async () => {
    const { fetch, context } = setup();
    fetch.empty(412);
    const error = await TestFetch.failure(context.api.delete('/routes/r1'));
    expect(error.status).toBe(412);
    expect(error.is(ProblemKind.PreconditionFailed)).toBe(true);
  });

  it('maps fetch failures to a network ApiError; silent requests are not published', async () => {
    const { context, recorder } = setup();
    const error = await TestFetch.failure(context.api.get('/routes', undefined, { silent: true }));
    expect(error.kind).toBe(ApiErrorKind.Network);
    expect(recorder.seen).toEqual([]);
  });
});
