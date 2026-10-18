import type { ActionIntent } from './intent';
import type { AlertState } from './alertRule';
import type { Tone } from './tone';

/** An entry of the top-bar Alerts bell (server alerts carry state, ack and time; mocks may omit them). */
export type Alert = { id: string; tone: Tone; title: string; detail: string; intent: ActionIntent; state?: AlertState; acked?: boolean; at?: string };

/** The bell feed: newest alerts plus the caller's unread (not acknowledged) count. */
export type AlertFeed = { items: Alert[]; unread: number };

/** A row of the dashboard "Needs attention" card. */
export type AttentionItem = {
  id: string;
  tone: Tone;
  kind: string;
  title: string;
  detail: string;
  actionLabel: string;
  intent: ActionIntent;
};
