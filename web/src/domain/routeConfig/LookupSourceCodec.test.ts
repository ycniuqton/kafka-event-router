import { describe, expect, it } from 'vitest';
import { LookupRequestKind } from '../lookupSource';
import { LookupErrorAction, LookupHttpMethod, LookupMissAction, LookupMockMode, LookupRedisOp, LookupStepDefaults, type LookupSourceStep } from '../lookupStep';
import { LookupMode, StepFactory, TransformStepKind, type TransformStep } from '../transform';
import { StepMapper } from './StepMapper';

/** Filled-in source-mode lookup steps, one per request kind, as the Studio form produces them. */
class LookupSourceSamples {
  static step(lookupSource: LookupSourceStep): TransformStep {
    return StepFactory.create(TransformStepKind.Lookup, { lookupMode: LookupMode.Source, lookupSource });
  }

  static readonly http: LookupSourceStep = {
    ...LookupStepDefaults.step(),
    sourceId: 'ls-crm-api',
    request: { ...LookupStepDefaults.request(LookupRequestKind.Http), method: LookupHttpMethod.Get, path: '/customers/${value.customerId}', query: [{ key: 'fields', value: 'tier' }] },
    map: [{ from: '$.tier', to: 'customer.tier' }],
    onMiss: { action: LookupMissAction.Default, value: '"standard"' },
    cache: { enabled: true, ttlMs: 60000, negativeTtlMs: 10000, maxEntries: 5000 },
    mockMode: LookupMockMode.Hit,
    mock: '{"tier":"gold"}',
  };

  static readonly redis: LookupSourceStep = {
    ...LookupStepDefaults.step(),
    sourceId: 'ls-flags',
    request: { ...LookupStepDefaults.request(LookupRequestKind.Redis), op: LookupRedisOp.HGet, key: 'flags:${value.merchantId}', hashField: 'checkout' },
    map: [{ from: '$', to: 'flags.checkout' }],
    timeoutMs: 200,
    retries: 0,
    onError: { action: LookupErrorAction.Skip, value: '' },
  };

  static readonly sql: LookupSourceStep = {
    ...LookupStepDefaults.step(),
    sourceId: 'ls-stores',
    request: { ...LookupStepDefaults.request(LookupRequestKind.Sql), sql: 'SELECT region FROM stores WHERE id = :id', params: [{ key: 'id', value: '${value.storeId}' }] },
    map: [{ from: '$.region', to: 'store.region' }],
    onMiss: { action: LookupMissAction.Fail, value: '' },
  };

  static readonly staticTable: LookupSourceStep = {
    ...LookupStepDefaults.step(),
    sourceId: 'ls-price-list',
    request: { ...LookupStepDefaults.request(LookupRequestKind.Static), key: '${value.country}' },
    map: [{ from: '$', to: 'region' }],
    onMiss: { action: LookupMissAction.Drop, value: '' },
  };

  static readonly all = [LookupSourceSamples.http, LookupSourceSamples.redis, LookupSourceSamples.sql, LookupSourceSamples.staticTable];
}

describe('LookupSourceCodec', () => {
  it('serialises the source mode to the engine lookup member (checked against the route schema by the Go test)', async () => {
    const steps = LookupSourceSamples.all.map((s) => StepMapper.toStep(LookupSourceSamples.step(s)));
    await expect(JSON.stringify(steps, null, 2) + '\n').toMatchFileSnapshot('./testdata/lookup_source_steps.json');
  });

  it('writes only the members of the request kind, document target paths, and no mock', () => {
    const doc = StepMapper.toStep(LookupSourceSamples.step(LookupSourceSamples.http));
    expect(doc).toEqual({
      kind: 'lookup',
      lookup: {
        sourceId: 'ls-crm-api',
        request: { kind: 'http', method: 'GET', path: '/customers/${value.customerId}', query: { fields: 'tier' } },
        map: [{ from: '$.tier', to: 'value.customer.tier' }],
        onMiss: { action: 'default', value: 'standard' },
        cache: { ttlMs: 60000, negativeTtlMs: 10000, maxEntries: 5000 },
      },
    });
    expect(JSON.stringify(doc)).not.toContain('gold');
  });

  it('reads every serialised source step back into the same form', () => {
    for (const sample of LookupSourceSamples.all) {
      const doc = StepMapper.toStep(LookupSourceSamples.step(sample));
      const back = StepMapper.fromStep(doc) as TransformStep;
      expect(back.lookupMode).toBe(LookupMode.Source);
      expect(back.lookupSource?.mockMode).toBe(LookupMockMode.Off);
      expect(StepMapper.toStep(back)).toEqual(doc);
    }
  });

  it('keeps table-mode lookups unchanged', () => {
    const step = StepFactory.create(TransformStepKind.Lookup, { field: 'country', lookupMode: LookupMode.Table, lookupRef: 'country-to-region', lookupVersion: 4 });
    expect(StepMapper.toStep(step)).toEqual({ kind: 'lookup', field: 'value.country', ref: 'country-to-region', version: 4 });
  });
});
