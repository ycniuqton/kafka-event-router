import { BuilderStep, IntentType, RouteDetailTab, Tone, type Alert, type AttentionItem } from '../domain';

/** Top-bar alerts (design `alerts`). */
export const ALERTS: Alert[] = [
  { id: 'al1', tone: Tone.Err, title: 'Order totals → analytics stopped', detail: 'JS step failed · 3 dead letters', intent: { type: IntentType.RouteDetail, routeId: 'r3', tab: RouteDetailTab.Dead } },
  { id: 'al2', tone: Tone.Warn, title: 'Clickstream merge lag growing', detail: '48,210 behind · up 12% in 15 min', intent: { type: IntentType.RouteDetail, routeId: 'r4', tab: RouteDetailTab.Overview } },
  { id: 'al3', tone: Tone.Err, title: 'partner-kafka auth failed', detail: 'Broker expects SCRAM-SHA-512', intent: { type: IntentType.TestConnection, connectionId: 'partner-kafka' } },
];

/** Dashboard "Needs attention" rows (design `attention`). */
export const ATTENTION: AttentionItem[] = [
  { id: 'at1', tone: Tone.Err, kind: 'Error', title: 'Order totals → analytics', detail: "JS step: Cannot read 'reduce' of undefined on 3 events", actionLabel: 'Open', intent: { type: IntentType.RouteDetail, routeId: 'r3', tab: RouteDetailTab.Dead } },
  { id: 'at2', tone: Tone.Warn, kind: 'Lag ↑', title: 'Clickstream merge', detail: 'Lag 48,210 and growing for 15 min', actionLabel: 'Pause', intent: { type: IntentType.TogglePause, routeId: 'r4' } },
  { id: 'at3', tone: Tone.Err, kind: 'Connection', title: 'partner-kafka', detail: 'Auth failed · 2 routes affected', actionLabel: 'Test', intent: { type: IntentType.TestConnection, connectionId: 'partner-kafka' } },
  { id: 'at4', tone: Tone.Warn, kind: '0 matches', title: 'Refunds → finance', detail: 'Filter matched 0 events in the last 30 min', actionLabel: 'Open', intent: { type: IntentType.RouteEdit, routeId: 'r2', step: BuilderStep.Filter } },
];
