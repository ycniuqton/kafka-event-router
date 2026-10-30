import { describe, expect, it } from 'vitest';
import { LookupMockMode, LookupMode, LookupRedisOp, LookupRequestKind, LookupStepDefaults, StepFactory, TransformStepKind, type SampleEvent } from '../../../../domain';
import { StudioRunPlan } from '../preview/StudioRunPlan';
import { LookupRequestFormSpec } from './LookupRequestFormSpec';
import { LookupSourceCatalog } from './LookupSourceCatalog';
import { StepPresenters } from './StepPresenters';
import { LOOKUP_SOURCES } from '../../../../mocks';

describe('LookupRequestFormSpec', () => {
  it('shows the inputs of the request kind and the redis command', () => {
    const members = (kind: LookupRequestKind, op: LookupRedisOp = LookupRedisOp.Get) => LookupRequestFormSpec.inputs({ ...LookupStepDefaults.request(kind), op }).map((i) => i.member);
    expect(members(LookupRequestKind.Http)).toEqual(['method', 'path', 'query', 'headers', 'body']);
    expect(members(LookupRequestKind.Redis)).toEqual(['op', 'key']);
    expect(members(LookupRequestKind.Redis, LookupRedisOp.HGet)).toEqual(['op', 'key', 'hashField']);
    expect(members(LookupRequestKind.Redis, LookupRedisOp.MGet)).toEqual(['op', 'keys']);
    expect(members(LookupRequestKind.Sql)).toEqual(['sql', 'params']);
    expect(members(LookupRequestKind.Static)).toEqual(['key']);
  });
});

describe('lookup source steps in the Studio', () => {
  const source = {
    ...LookupStepDefaults.step(),
    sourceId: 'ls-crm-api',
    request: { ...LookupStepDefaults.request(LookupRequestKind.Http), path: '/c/${value.id}' },
    map: [{ from: '$.tier', to: 'customer.tier' }],
  };
  const sample = { id: 1, topic: 'orders', key: null, headers: {}, value: { id: 7 } } as unknown as SampleEvent;

  it('sends mocks keyed by the step id it assigns, and no mocks for live steps', () => {
    const mocked = StepFactory.create(TransformStepKind.Lookup, { lookupMode: LookupMode.Source, lookupSource: { ...source, mockMode: LookupMockMode.Hit, mock: '{"tier":"gold"}' } });
    const live = StepFactory.create(TransformStepKind.Lookup, { lookupMode: LookupMode.Source, lookupSource: source });
    const plan = new StudioRunPlan([live, mocked], [sample]);
    expect(plan.request.lookupMocks).toEqual({ 'lookup-1': { found: true, value: { tier: 'gold' } } });
    const steps = (plan.request.config as { branches: { transform: { steps: { id?: string }[] } }[] }).branches[0].transform.steps;
    expect(steps.map((s) => s.id)).toEqual([undefined, 'lookup-1']);
    expect(new StudioRunPlan([live], [sample]).request.lookupMocks).toBeUndefined();
  });

  it('summarises the step with the source name and kind once the catalog is loaded', () => {
    const step = StepFactory.create(TransformStepKind.Lookup, { lookupMode: LookupMode.Source, lookupSource: source });
    LookupSourceCatalog.load([]);
    expect(StepPresenters.of(step.kind).summary(step)).toBe('ls-crm-api → customer.tier');
    LookupSourceCatalog.load(LOOKUP_SOURCES);
    expect(StepPresenters.of(step.kind).summary(step)).toBe('crm-api (HTTP) → customer.tier');
  });
});
