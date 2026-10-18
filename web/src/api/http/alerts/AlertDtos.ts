import type { ActionIntent, AlertRuleKind, AlertState, Tone } from '../../../domain';

/** Paths of the alerting endpoints (relative to `/api/v1`). */
export const AlertsEndpoint = {
  attention: '/dashboard/attention',
  feed: '/alerts',
  ack: (id: string) => `/alerts/${encodeURIComponent(id)}/ack`,
  rules: '/alert-rules',
  rule: (id: string) => `/alert-rules/${encodeURIComponent(id)}`,
  test: (id: string) => `/alert-rules/${encodeURIComponent(id)}/test`,
} as const;

/** Query parameters of the alerting endpoints. */
export const AlertsParam = { routeId: 'routeId', limit: 'limit' } as const;

/** Page size of the bell feed. */
export const AlertsFeedLimit = 20;

/** One `GET /dashboard/attention` row. */
export type AttentionItemDto = {
  id: string;
  kind: string;
  tone: Tone;
  title: string;
  detail: string;
  action: { label: string; intent: ActionIntent };
};

export type AttentionDto = { items: AttentionItemDto[] };

/** One `GET /alerts` row. */
export type AlertFeedItemDto = {
  id: number;
  ruleId: string;
  ruleName: string;
  routeId: string;
  routeName: string;
  kind: AlertRuleKind;
  threshold: number;
  windowMinutes: number;
  state: AlertState;
  value: number | null;
  firedAt: string;
  resolvedAt: string | null;
  acked: boolean;
  intent: ActionIntent;
};

export type AlertFeedDto = { items: AlertFeedItemDto[]; next_cursor: string | null; unread: number };
