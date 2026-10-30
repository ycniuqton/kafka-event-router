/** Source mode of the lookup step (pipeline-engine 00002-lookups): request templates, answer mapping, policies, mock. */
import { LookupRequestKind, LookupSourceKind, LookupSourceKindMeta } from './lookupSource';
import { Tone } from './tone';

export const LookupHttpMethod = { Get: 'GET', Post: 'POST' } as const;
export type LookupHttpMethod = (typeof LookupHttpMethod)[keyof typeof LookupHttpMethod];

export const LookupRedisOp = { Get: 'get', HGet: 'hget', MGet: 'mget' } as const;
export type LookupRedisOp = (typeof LookupRedisOp)[keyof typeof LookupRedisOp];

export const LookupMissAction = { Default: 'default', Skip: 'skip', Drop: 'drop', Fail: 'fail' } as const;
export type LookupMissAction = (typeof LookupMissAction)[keyof typeof LookupMissAction];

export const LookupErrorAction = { Fail: 'fail', Skip: 'skip', Default: 'default' } as const;
export type LookupErrorAction = (typeof LookupErrorAction)[keyof typeof LookupErrorAction];

/** Studio-only mock of a step's answer: off (live), a found value, or a miss. */
export const LookupMockMode = { Off: 'off', Hit: 'hit', Miss: 'miss' } as const;
export type LookupMockMode = (typeof LookupMockMode)[keyof typeof LookupMockMode];

/** One name → template row (HTTP query / headers, SQL params). */
export type LookupKeyValue = { key: string; value: string };
/** One answer pick (`$.tier`) → target field (value-relative, like other Studio steps). */
export type LookupMapRow = { from: string; to: string };

export type LookupRequestDraft = {
  kind: LookupRequestKind;
  method: LookupHttpMethod;
  path: string;
  query: LookupKeyValue[];
  headers: LookupKeyValue[];
  /** HTTP JSON body as text (empty = none). */
  body: string;
  op: LookupRedisOp;
  key: string;
  keys: string[];
  hashField: string;
  sql: string;
  params: LookupKeyValue[];
};

export type LookupPolicyDraft<A extends string> = { action: A; value: string };
export type LookupCacheDraft = { enabled: boolean; ttlMs: number; negativeTtlMs: number; maxEntries: number };

export type LookupSourceStep = {
  sourceId: string;
  request: LookupRequestDraft;
  map: LookupMapRow[];
  timeoutMs: number;
  retries: number;
  onMiss: LookupPolicyDraft<LookupMissAction>;
  onError: LookupPolicyDraft<LookupErrorAction>;
  cache: LookupCacheDraft;
  mockMode: LookupMockMode;
  /** Mock answer as JSON text (used when `mockMode` is hit). */
  mock: string;
};

/** Engine defaults of absent members (00002 §1). */
export class LookupStepDefaults {
  static readonly timeoutMs = 500;
  static readonly retries = 1;
  static readonly maxEntries = 10000;
  static readonly ttlMs = 60000;

  static request(kind: LookupRequestKind = LookupRequestKind.Http): LookupRequestDraft {
    return { kind, method: LookupHttpMethod.Get, path: '', query: [], headers: [], body: '', op: LookupRedisOp.Get, key: '', keys: [], hashField: '', sql: '', params: [] };
  }

  static step(): LookupSourceStep {
    return {
      sourceId: '',
      request: LookupStepDefaults.request(),
      map: [{ from: AnswerPick.root, to: '' }],
      timeoutMs: LookupStepDefaults.timeoutMs,
      retries: LookupStepDefaults.retries,
      onMiss: { action: LookupMissAction.Skip, value: '' },
      onError: { action: LookupErrorAction.Fail, value: '' },
      cache: { enabled: false, ttlMs: LookupStepDefaults.ttlMs, negativeTtlMs: 0, maxEntries: LookupStepDefaults.maxEntries },
      mockMode: LookupMockMode.Off,
      mock: '',
    };
  }

  /** The step after picking a source: the request kind follows the source kind. */
  static withSource(step: LookupSourceStep, sourceId: string, kind: LookupSourceKind): LookupSourceStep {
    const requestKind = LookupSourceKindMeta[kind].requestKind;
    const request = step.request.kind === requestKind ? step.request : LookupStepDefaults.request(requestKind);
    return { ...step, sourceId, request };
  }
}

export const LookupMissActionLabel: Record<LookupMissAction, string> = {
  [LookupMissAction.Skip]: 'Keep the event as is',
  [LookupMissAction.Default]: 'Write a default value',
  [LookupMissAction.Drop]: 'Drop the event',
  [LookupMissAction.Fail]: 'Fail (dead-letter)',
};

export const LookupErrorActionLabel: Record<LookupErrorAction, string> = {
  [LookupErrorAction.Fail]: 'Fail (error policy)',
  [LookupErrorAction.Skip]: 'Keep the event as is',
  [LookupErrorAction.Default]: 'Write a default value',
};

/** JSONPath-ish answer picks: `$`, `$.a.b`, `$.items[0].name`. */
export class AnswerPick {
  static readonly root = '$';
  static readonly pattern = /\.([^.[\]]+)|\[(\d+)\]/g;

  static valid(pick: string): boolean {
    if (!pick.startsWith(AnswerPick.root)) return false;
    return pick.slice(AnswerPick.root.length).replace(AnswerPick.pattern, '') === '';
  }

  static read(answer: unknown, pick: string): unknown {
    if (!AnswerPick.valid(pick)) return undefined;
    let node: unknown = answer;
    for (const m of pick.slice(AnswerPick.root.length).matchAll(AnswerPick.pattern)) {
      if (node == null || typeof node !== 'object') return undefined;
      node = (node as Record<string, unknown>)[m[1] ?? Number(m[2])];
    }
    return node;
  }
}

/** Mode shown on a preview step from its lookup log line. */
export const LookupTraceMode = { Live: 'live', Mock: 'mock', Cache: 'cache' } as const;
export type LookupTraceMode = (typeof LookupTraceMode)[keyof typeof LookupTraceMode];

export const LookupTraceResult = { Hit: 'hit', Miss: 'miss', Error: 'error' } as const;
export type LookupTraceResult = (typeof LookupTraceResult)[keyof typeof LookupTraceResult];

export type LookupTrace = { mode: LookupTraceMode; source: string; result: LookupTraceResult; ms: number };

/** Parses the engine's `lookup: mode=<m> source=<id> result=<r> ms=<n>` step log line. */
export class LookupTraceLine {
  static readonly prefix = 'lookup:';
  static readonly pair = /(\w+)=(\S+)/g;

  static parse(line: string): LookupTrace | null {
    if (!line.startsWith(LookupTraceLine.prefix)) return null;
    const fields = new Map([...line.slice(LookupTraceLine.prefix.length).matchAll(LookupTraceLine.pair)].map((m) => [m[1], m[2]]));
    const mode = fields.get('mode') as LookupTraceMode | undefined;
    if (!mode || !Object.values(LookupTraceMode).includes(mode)) return null;
    const result = (fields.get('result') ?? LookupTraceResult.Hit) as LookupTraceResult;
    return { mode, source: fields.get('source') ?? '', result, ms: Number(fields.get('ms') ?? 0) };
  }

  static first(lines: readonly string[] | undefined): LookupTrace | null {
    for (const line of lines ?? []) {
      const trace = LookupTraceLine.parse(line);
      if (trace) return trace;
    }
    return null;
  }

  static format(t: LookupTrace): string {
    return `${LookupTraceLine.prefix} mode=${t.mode} source=${t.source} result=${t.result} ms=${t.ms}`;
  }
}

export const LookupTraceBadge: Record<LookupTraceMode, { label: string; tone: Tone }> = {
  [LookupTraceMode.Live]: { label: 'live lookup', tone: Tone.Info },
  [LookupTraceMode.Mock]: { label: 'mock', tone: Tone.Warn },
  [LookupTraceMode.Cache]: { label: 'cached', tone: Tone.Off },
};

/** What a lookup source step still lacks before it can run. */
export const LookupStepIssue = {
  Source: 'Pick a lookup source.',
  Request: 'Fill in the request (path, key or SQL).',
  Mapping: 'Map the answer into at least one field.',
  Pick: 'Answer picks start with $ (e.g. $.tier).',
  Mock: 'The mock answer is not valid JSON.',
} as const;
export type LookupStepIssue = (typeof LookupStepIssue)[keyof typeof LookupStepIssue];

/** Completeness of the request per request kind (registry). */
export class LookupStepRules {
  private static readonly requestReady: Record<LookupRequestKind, (r: LookupRequestDraft) => boolean> = {
    [LookupRequestKind.Http]: (r) => !!r.path.trim(),
    [LookupRequestKind.Redis]: (r) => (r.op === LookupRedisOp.MGet ? r.keys.some((k) => k.trim()) : !!r.key.trim() && (r.op !== LookupRedisOp.HGet || !!r.hashField.trim())),
    [LookupRequestKind.Sql]: (r) => !!r.sql.trim(),
    [LookupRequestKind.Static]: (r) => !!r.key.trim(),
  };

  static issue(step: LookupSourceStep): LookupStepIssue | null {
    if (!step.sourceId) return LookupStepIssue.Source;
    if (!LookupStepRules.requestReady[step.request.kind](step.request)) return LookupStepIssue.Request;
    const rows = step.map.filter((m) => m.to.trim());
    if (!rows.length) return LookupStepIssue.Mapping;
    if (rows.some((m) => !AnswerPick.valid(m.from.trim()))) return LookupStepIssue.Pick;
    if (step.mockMode === LookupMockMode.Hit && MockAnswer.parse(step.mock) === undefined) return LookupStepIssue.Mock;
    return null;
  }
}

/** The mock answer text of a step: JSON when it parses, undefined when invalid. */
export class MockAnswer {
  static parse(text: string): unknown {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return undefined;
    }
  }
}

/** One preview mock as `POST /preview/run` takes it (`lookupMocks[stepId]`). */
export type LookupMockDto = { found: boolean; value?: unknown };
export type LookupMocks = Record<string, LookupMockDto>;

/** Studio mocks ↔ run request: lookup steps with a mock get the step id `lookup-<runIndex>` and an entry in `lookupMocks`. */
export class LookupMockPlan {
  static readonly idPrefix = 'lookup-';

  static idOf(runIndex: number): string {
    return `${LookupMockPlan.idPrefix}${runIndex}`;
  }

  /** The mock a step sends (null = call the live source). */
  static mockOf(step: LookupSourceStep): LookupMockDto | null {
    if (step.mockMode === LookupMockMode.Miss) return { found: false };
    if (step.mockMode !== LookupMockMode.Hit) return null;
    const value = MockAnswer.parse(step.mock);
    return value === undefined ? null : { found: true, value };
  }

  /** The step with a received mock applied (mock preview service). */
  static applyTo(step: LookupSourceStep, mock: LookupMockDto): LookupSourceStep {
    if (!mock.found) return { ...step, mockMode: LookupMockMode.Miss };
    return { ...step, mockMode: LookupMockMode.Hit, mock: JSON.stringify(mock.value ?? null) };
  }
}
