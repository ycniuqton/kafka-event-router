import { describe, expect, it } from 'vitest';
import { LookupAnswerApplier, LookupMissError } from './pipeline/LookupAnswerApplier';
import { LookupRequestKind, LookupSourceKind } from './lookupSource';
import {
  AnswerPick,
  LookupMissAction,
  LookupMockMode,
  LookupMockPlan,
  LookupRedisOp,
  LookupStepDefaults,
  LookupStepIssue,
  LookupStepRules,
  LookupTraceLine,
  LookupTraceMode,
  LookupTraceResult,
  type LookupSourceStep,
} from './lookupStep';
import type { KafkaEvent } from './event';

const ready = (patch: Partial<LookupSourceStep> = {}): LookupSourceStep => ({
  ...LookupStepDefaults.step(),
  sourceId: 'ls-crm-api',
  request: { ...LookupStepDefaults.request(LookupRequestKind.Http), path: '/c/${value.id}' },
  map: [{ from: '$.tier', to: 'tier' }],
  ...patch,
});
const event = (): KafkaEvent => ({ key: '', headers: {}, value: { id: 7 } }) as unknown as KafkaEvent;

describe('AnswerPick', () => {
  it('reads $, members and indexes; rejects picks without $', () => {
    const answer = { a: { b: [{ name: 'x' }] }, tier: 'gold' };
    expect(AnswerPick.read(answer, '$')).toEqual(answer);
    expect(AnswerPick.read(answer, '$.tier')).toBe('gold');
    expect(AnswerPick.read(answer, '$.a.b[0].name')).toBe('x');
    expect(AnswerPick.read(answer, '$.missing.x')).toBeUndefined();
    expect(AnswerPick.valid('tier')).toBe(false);
    expect(AnswerPick.valid('$.a..b')).toBe(false);
  });
});

describe('LookupTraceLine', () => {
  it('parses the engine log line and round-trips its own format', () => {
    expect(LookupTraceLine.parse('lookup: mode=live source=ls-1 result=hit ms=34')).toEqual({ mode: LookupTraceMode.Live, source: 'ls-1', result: LookupTraceResult.Hit, ms: 34 });
    expect(LookupTraceLine.parse('hello')).toBeNull();
    expect(LookupTraceLine.parse('lookup: mode=bogus')).toBeNull();
    const trace = { mode: LookupTraceMode.Cache, source: 's', result: LookupTraceResult.Miss, ms: 0 };
    expect(LookupTraceLine.parse(LookupTraceLine.format(trace))).toEqual(trace);
    expect(LookupTraceLine.first(['x', LookupTraceLine.format(trace)])).toEqual(trace);
  });
});

describe('LookupStepRules', () => {
  it('names the first missing piece of a source step', () => {
    expect(LookupStepRules.issue(LookupStepDefaults.step())).toBe(LookupStepIssue.Source);
    expect(LookupStepRules.issue(ready({ request: LookupStepDefaults.request(LookupRequestKind.Http) }))).toBe(LookupStepIssue.Request);
    expect(LookupStepRules.issue(ready({ map: [{ from: '$', to: ' ' }] }))).toBe(LookupStepIssue.Mapping);
    expect(LookupStepRules.issue(ready({ map: [{ from: 'tier', to: 'x' }] }))).toBe(LookupStepIssue.Pick);
    expect(LookupStepRules.issue(ready({ mockMode: LookupMockMode.Hit, mock: '{oops' }))).toBe(LookupStepIssue.Mock);
    expect(LookupStepRules.issue(ready())).toBeNull();
  });

  it('checks redis requests by command', () => {
    const redis = (patch: object) => ready({ request: { ...LookupStepDefaults.request(LookupRequestKind.Redis), ...patch } });
    expect(LookupStepRules.issue(redis({ op: LookupRedisOp.HGet, key: 'k' }))).toBe(LookupStepIssue.Request);
    expect(LookupStepRules.issue(redis({ op: LookupRedisOp.HGet, key: 'k', hashField: 'f' }))).toBeNull();
    expect(LookupStepRules.issue(redis({ op: LookupRedisOp.MGet, keys: ['a'] }))).toBeNull();
  });

  it('switches the request kind with the source kind', () => {
    const step = LookupStepDefaults.withSource(ready(), 'ls-db', LookupSourceKind.Postgres);
    expect(step.request.kind).toBe(LookupRequestKind.Sql);
    expect(LookupStepDefaults.withSource(ready(), 'ls-2', LookupSourceKind.Http).request.path).toBe('/c/${value.id}');
  });
});

describe('LookupMockPlan + LookupAnswerApplier', () => {
  it('turns Studio mocks into run mocks and back', () => {
    expect(LookupMockPlan.mockOf(ready())).toBeNull();
    expect(LookupMockPlan.mockOf(ready({ mockMode: LookupMockMode.Miss }))).toEqual({ found: false });
    expect(LookupMockPlan.mockOf(ready({ mockMode: LookupMockMode.Hit, mock: '{"tier":"gold"}' }))).toEqual({ found: true, value: { tier: 'gold' } });
    expect(LookupMockPlan.applyTo(ready(), { found: true, value: 1 })).toMatchObject({ mockMode: LookupMockMode.Hit, mock: '1' });
  });

  it('applies a mock answer through the mapping and logs a mock line', () => {
    const logs: string[] = [];
    const out = LookupAnswerApplier.apply(event(), ready({ mockMode: LookupMockMode.Hit, mock: '{"tier":"gold"}' }), logs);
    expect(out?.value).toEqual({ id: 7, tier: 'gold' });
    expect(LookupTraceLine.first(logs)?.mode).toBe(LookupTraceMode.Mock);
  });

  it('applies onMiss to a mocked miss and passes through without a mock', () => {
    const miss = (action: LookupMissAction, value = '') => LookupAnswerApplier.apply(event(), ready({ mockMode: LookupMockMode.Miss, onMiss: { action, value } }), []);
    expect(miss(LookupMissAction.Default, '"std"')?.value).toEqual({ id: 7, tier: 'std' });
    expect(miss(LookupMissAction.Skip)?.value).toEqual({ id: 7 });
    expect(miss(LookupMissAction.Drop)).toBeNull();
    expect(() => miss(LookupMissAction.Fail)).toThrow(LookupMissError);
    expect(LookupAnswerApplier.apply(event(), ready(), [])?.value).toEqual({ id: 7 });
  });
});
