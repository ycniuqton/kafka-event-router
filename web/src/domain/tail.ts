import { Tone } from './tone';

/** How a tailed record finished (server `TailOutcome`). */
export const TailOutcome = {
  Sent: 'sent',
  Filtered: 'filtered',
  Dropped: 'dropped',
  DeadLettered: 'dead_lettered',
  Error: 'error',
} as const;
export type TailOutcome = (typeof TailOutcome)[keyof typeof TailOutcome];

/** How a tailed payload is carried. */
export const TailEncoding = { Json: 'json', Text: 'text', Base64: 'base64' } as const;
export type TailEncoding = (typeof TailEncoding)[keyof typeof TailEncoding];

export type TailHeader = { key: string; value: string };

/** One input or output record of a tail entry. */
export type TailEvent = {
  topic?: string;
  partition: number;
  offset: number;
  branch?: string;
  key?: unknown;
  value: unknown;
  encoding: TailEncoding;
  headers?: TailHeader[];
  truncated?: boolean;
};

/** One finished source record with its outputs (`WS /routes/{id}/tail` record frame). */
export type TailRecord = {
  at: string;
  partition: number;
  offset: number;
  branch?: string;
  outcome: TailOutcome;
  step?: string;
  errorType?: string;
  error?: string;
  input: TailEvent;
  outputs: TailEvent[];
};

/** Stream states reported by status frames, plus client-side connection states. */
export const TailState = {
  Connecting: 'connecting',
  Live: 'live',
  Paused: 'paused',
  Throttled: 'throttled',
  NotRunning: 'not_running',
  Unavailable: 'unavailable',
  Reconnecting: 'reconnecting',
  Closed: 'closed',
} as const;
export type TailState = (typeof TailState)[keyof typeof TailState];

/** Frame types the server sends. */
export const TailFrameType = { Record: 'record', Status: 'status' } as const;
export type TailFrameType = (typeof TailFrameType)[keyof typeof TailFrameType];

export type TailFrame =
  | { type: typeof TailFrameType.Record; record: TailRecord; masked: boolean }
  | { type: typeof TailFrameType.Status; state: TailState; masked: boolean };

/** Ops the client sends. */
export const TailOp = { Pause: 'pause', Resume: 'resume', Filter: 'filter' } as const;
export type TailOp = (typeof TailOp)[keyof typeof TailOp];

export type TailClientOp = { op: TailOp; outcomes?: TailOutcome[]; text?: string };

/** What the viewer filters by: "show only" choice and free text (sent to the server and applied locally). */
export const TailShow = { All: 'all', Dropped: 'dropped', Errors: 'errors' } as const;
export type TailShow = (typeof TailShow)[keyof typeof TailShow];

export type TailFilter = { show: TailShow; text: string };

/** Close codes of the tail socket. */
export const TailCloseCode = { Normal: 1000, Unavailable: 1011, NotRunning: 4409 } as const;

/** Limits of the live tail view. */
export const TailLimits = { maxRows: 500, ratePerSec: 20 } as const;

/** Icons, tones and labels of tail outcomes. */
export const TailOutcomeMeta: Record<TailOutcome, { icon: string; tone: Tone; label: string }> = {
  [TailOutcome.Sent]: { icon: '✓', tone: Tone.Ok, label: 'sent' },
  [TailOutcome.Filtered]: { icon: '⊘', tone: Tone.Off, label: 'filtered' },
  [TailOutcome.Dropped]: { icon: '⊘', tone: Tone.Warn, label: 'dropped' },
  [TailOutcome.DeadLettered]: { icon: '⚑', tone: Tone.Err, label: 'dead-lettered' },
  [TailOutcome.Error]: { icon: '✕', tone: Tone.Err, label: 'error' },
};

/** Rules of the show-only choices: which outcomes each one keeps. */
export class TailShowRules {
  static readonly options = [
    { value: TailShow.All, label: 'All' },
    { value: TailShow.Dropped, label: 'Only dropped' },
    { value: TailShow.Errors, label: 'Only errors' },
  ] as const;

  private static readonly outcomes: Record<TailShow, TailOutcome[]> = {
    [TailShow.All]: [],
    [TailShow.Dropped]: [TailOutcome.Filtered, TailOutcome.Dropped],
    [TailShow.Errors]: [TailOutcome.Error, TailOutcome.DeadLettered],
  };

  static outcomesOf(show: TailShow): TailOutcome[] {
    return TailShowRules.outcomes[show];
  }

  static op(filter: TailFilter): TailClientOp {
    return { op: TailOp.Filter, outcomes: TailShowRules.outcomesOf(filter.show), text: filter.text.trim() };
  }

  static matches(record: TailRecord, filter: TailFilter): boolean {
    const outcomes = TailShowRules.outcomesOf(filter.show);
    if (outcomes.length > 0 && !outcomes.includes(record.outcome)) return false;
    const needle = filter.text.trim().toLowerCase();
    return !needle || JSON.stringify(record).toLowerCase().includes(needle);
  }
}

/** Headline and tone of each tail state. */
export const TailStateMeta: Record<TailState, { label: string; tone: Tone; live: boolean }> = {
  [TailState.Connecting]: { label: 'Connecting…', tone: Tone.Muted, live: false },
  [TailState.Live]: { label: 'Streaming', tone: Tone.Ok, live: true },
  [TailState.Paused]: { label: 'Stream paused', tone: Tone.Off, live: false },
  [TailState.Throttled]: { label: 'Streaming · throttled to 20 events/s', tone: Tone.Warn, live: true },
  [TailState.NotRunning]: { label: 'Route is not running', tone: Tone.Off, live: false },
  [TailState.Unavailable]: { label: 'Live tail unavailable', tone: Tone.Err, live: false },
  [TailState.Reconnecting]: { label: 'Reconnecting…', tone: Tone.Warn, live: false },
  [TailState.Closed]: { label: 'Stream closed', tone: Tone.Off, live: false },
};
