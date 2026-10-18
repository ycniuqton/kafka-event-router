import { describe, expect, it } from 'vitest';
import { AlertRuleKind, AlertState, ChannelType, IntentType, RouteDetailTab, Tone } from '../../../domain';
import { AlertFeedMapper } from './HttpAlertsService';
import { HttpAlertsService } from './HttpAlertsService';
import { HttpContext } from '../core/HttpContext';

const feedRow = {
  id: 7, ruleId: 'rule', ruleName: 'Lag high', routeId: 'r1', routeName: 'orders', kind: AlertRuleKind.LagAbove, threshold: 20000,
  windowMinutes: 10, state: AlertState.Firing, value: 25000, firedAt: '2026-10-14T09:00:00Z', resolvedAt: null, acked: false,
  intent: { type: IntentType.RouteDetail, routeId: 'r1', tab: RouteDetailTab.Overview },
} as const;

type Call = { method: string; url: string; body: string | null; ifMatch: string | null };

function service(responses: Record<string, unknown>): { svc: HttpAlertsService; calls: Call[] } {
  const calls: Call[] = [];
  const fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const headers = new Headers(init?.headers);
    calls.push({ method: init?.method ?? 'GET', url, body: (init?.body as string) ?? null, ifMatch: headers.get('If-Match') });
    const key = Object.keys(responses).find((k) => url.includes(k));
    const body = key ? responses[key] : null;
    return new Response(body === null ? null : JSON.stringify(body), { status: body === null ? 204 : 200, headers: { 'Content-Type': 'application/json' } });
  };
  return { svc: new HttpAlertsService(new HttpContext({ apiBaseUrl: '/api/v1', fetch })), calls };
}

describe('HttpAlertsService', () => {
  it('maps bell rows to a title, condition and current value', () => {
    expect(AlertFeedMapper.item(feedRow)).toMatchObject({
      id: '7', tone: Tone.Err, title: 'orders · Lag high', detail: 'Lag above 20,000 msgs for 10 min · now 25,000', state: AlertState.Firing, acked: false,
    });
    expect(AlertFeedMapper.item({ ...feedRow, state: AlertState.Resolved, value: null, resolvedAt: '2026-10-14T09:05:00Z' })).toMatchObject({
      tone: Tone.Ok, detail: 'Lag above 20,000 msgs for 10 min · now no data · resolved', at: '2026-10-14T09:05:00Z',
    });
  });

  it('reads the feed with its unread count', async () => {
    const { svc, calls } = service({ '/alerts': { items: [feedRow], next_cursor: null, unread: 1 } });
    const feed = await svc.feed();
    expect(feed.unread).toBe(1);
    expect(feed.items[0].title).toBe('orders · Lag high');
    expect(calls[0].url).toContain('/api/v1/alerts?limit=20');
  });

  it('creates with POST and updates with PATCH + If-Match without the route id', async () => {
    const rule = { id: 'x', routeId: 'r1', name: 'n', kind: AlertRuleKind.LagAbove, threshold: 5, windowMinutes: 5, enabled: true, version: 2, channels: [] };
    const { svc, calls } = service({ '/alert-rules': rule });
    const draft = { routeId: 'r1', name: 'n', kind: AlertRuleKind.LagAbove, threshold: 5, windowMinutes: 5, enabled: true, channels: [{ type: ChannelType.Webhook }] };
    await svc.saveRule({ draft });
    await svc.saveRule({ draft, existing: { id: 'x', version: 2 } });
    expect(calls[0]).toMatchObject({ method: 'POST', ifMatch: null });
    expect(calls[1]).toMatchObject({ method: 'PATCH', ifMatch: '"2"' });
    expect(calls[1].url).toContain('/alert-rules/x');
    expect(JSON.parse(calls[1].body ?? '{}')).not.toHaveProperty('routeId');
  });
});
