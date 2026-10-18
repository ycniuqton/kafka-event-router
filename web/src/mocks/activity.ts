import { ActivityVerb, type Activity } from '../domain';
import { MockTime } from './time';

/** Design `ACTIVITY` (newest first). */
export const ACTIVITY: Activity[] = [
  { id: 'a1', actor: 'Minh N.', verb: ActivityVerb.Deployed, routeName: 'Order paid → partner-gateway', detail: 'v4', at: MockTime.ago(12, 'min') },
  { id: 'a2', actor: 'Quincy N.', verb: ActivityVerb.Edited, routeName: 'Order totals → analytics', detail: 'JS step', at: MockTime.ago(41, 'min') },
  { id: 'a3', actor: 'Hoa L.', verb: ActivityVerb.Paused, routeName: 'Stock changes → warehouse', detail: '', at: MockTime.ago(2, 'h') },
  { id: 'a4', actor: 'Duc V.', verb: ActivityVerb.RolledBack, routeName: 'Shipment updates → partner', detail: 'to v8', at: MockTime.ago(6, 'h') },
  { id: 'a5', actor: 'Mai D.', verb: ActivityVerb.Created, routeName: 'Fraud signals', detail: 'draft', at: '2026-10-12T17:30:00.000Z' },
  { id: 'a6', actor: 'An P.', verb: ActivityVerb.Deployed, routeName: 'Clickstream merge', detail: 'v3', at: MockTime.ago(1, 'd') },
];
