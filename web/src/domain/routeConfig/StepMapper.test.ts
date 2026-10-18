import { describe, expect, it } from 'vitest';
import { FilterCombinator, FilterMode, FilterOp } from '../filter';
import { LookupMode, MaskMode, StepFactory, TransformStepKind, type TransformStep } from '../transform';
import { ConvertFailure, ConvertTarget, MetadataName, ValueSourceKind } from '../transformOptions';
import { StepMapper } from './StepMapper';
import { MapStart } from '../mapper';

/** One filled-in form per Studio recipe kind, as the recipe forms produce them. */
class RecipeFormSamples {
  static readonly all: Record<TransformStepKind, Partial<TransformStep>> = {
    [TransformStepKind.Rename]: { field: 'cust_id', argument: 'customerId' },
    [TransformStepKind.Remove]: { fields: ['phone', 'internal.notes'], field: 'phone' },
    [TransformStepKind.KeepOnly]: { fields: ['orderId', 'amount'], field: 'orderId' },
    [TransformStepKind.Copy]: { field: 'orderId', argument: 'meta.orderRef' },
    [TransformStepKind.Move]: { field: 'customer.email', argument: 'contact.email' },
    [TransformStepKind.Mask]: { field: 'email', maskMode: MaskMode.Email },
    [TransformStepKind.Hash]: { field: 'cust_id' },
    [TransformStepKind.ConvertType]: { field: 'amount', targetType: ConvertTarget.Integer, onFailure: ConvertFailure.Null },
    [TransformStepKind.Set]: { field: 'source', sourceKind: ValueSourceKind.Metadata, argument: MetadataName.Topic },
    [TransformStepKind.DefaultIfMissing]: { field: 'currency', sourceKind: ValueSourceKind.Constant, argument: 'EUR' },
    [TransformStepKind.DropIf]: {
      condition: { combinator: FilterCombinator.Or, mode: FilterMode.Visual, expression: '', rows: [{ field: 'value.amount', op: FilterOp.LessThan, value: '0' }, { field: 'value.test', op: FilterOp.Exists, value: '' }] },
    },
    [TransformStepKind.SetKey]: { field: 'orderId', sourceKind: ValueSourceKind.Field },
    [TransformStepKind.Map]: {
      mapStart: MapStart.CopyInput,
      mapEntries: [
        { path: 'customer.id', sourceKind: ValueSourceKind.Field, argument: 'cust_id' },
        { path: 'cust_id', sourceKind: ValueSourceKind.Constant, argument: '', remove: true },
        { path: 'total', sourceKind: ValueSourceKind.Formula, argument: 'value.amount * 2' },
        { path: 'meta.topic', sourceKind: ValueSourceKind.Metadata, argument: MetadataName.Topic },
        { path: 'meta.version', sourceKind: ValueSourceKind.Constant, argument: '2' },
      ],
    },
    [TransformStepKind.Lookup]: {
      field: 'country',
      argument: 'region',
      fallback: '"OTHER"',
      lookupMode: LookupMode.Inline,
      lookupRows: [{ key: 'VN', value: 'APAC' }, { key: 'DE', value: { region: 'EMEA' } }],
    },
    [TransformStepKind.SplitArray]: { field: 'items', argument: 'item' },
    [TransformStepKind.Wrap]: { field: 'payload' },
    [TransformStepKind.Unwrap]: { field: 'data.order' },
    [TransformStepKind.HeaderAdd]: { header: 'source', sourceKind: ValueSourceKind.Metadata, argument: MetadataName.Topic },
    [TransformStepKind.HeaderRemove]: { header: 'x-internal' },
    [TransformStepKind.HeaderRename]: { header: 'traceId', argument: 'trace-id' },
    [TransformStepKind.HeaderToPayload]: { header: 'traceId', field: 'meta.traceId' },
    [TransformStepKind.HeaderFromPayload]: { field: 'orderId', header: 'order-id' },
    [TransformStepKind.Library]: { libraryRef: 'mask-customer-pii', libraryVersion: 3 },
    [TransformStepKind.Js]: { code: 'function transform(event, ctx) { return event; }' },
  };

  static step(kind: TransformStepKind): TransformStep {
    return StepFactory.create(kind, RecipeFormSamples.all[kind]);
  }
}

describe('StepMapper', () => {
  it('serialises every recipe form to an engine step (checked against the route schema by the Go test)', async () => {
    const steps = Object.values(TransformStepKind).map((kind) => StepMapper.toStep(RecipeFormSamples.step(kind)));
    await expect(JSON.stringify(steps, null, 2) + '\n').toMatchFileSnapshot('./testdata/studio_recipe_steps.json');
  });

  it('reads every serialised recipe back into the same form', () => {
    for (const kind of Object.values(TransformStepKind)) {
      const original = RecipeFormSamples.step(kind);
      const back = StepMapper.fromStep(StepMapper.toStep(original));
      expect(back?.kind).toBe(kind);
      expect(StepMapper.toStep(back as TransformStep)).toEqual(StepMapper.toStep(original));
    }
  });

  it('maps named lookup tables to a pinned ref and back', () => {
    const step = StepFactory.create(TransformStepKind.Lookup, { field: 'country', lookupMode: LookupMode.Table, lookupRef: 'country-to-region', lookupVersion: 4 });
    const doc = StepMapper.toStep(step);
    expect(doc).toEqual({ kind: 'lookup', field: 'value.country', ref: 'country-to-region', version: 4 });
    expect(StepMapper.fromStep(doc)).toMatchObject({ lookupMode: LookupMode.Table, lookupRef: 'country-to-region', lookupVersion: 4, argument: '' });
  });

  it('types fixed values as JSON literals and keeps other text a string', () => {
    const set = (argument: string) => StepMapper.toStep(StepFactory.create(TransformStepKind.Set, { field: 'x', argument })).source?.value;
    expect([set('42'), set('true'), set('"42"'), set('hello'), set('{"a":1}')]).toEqual([42, true, '42', 'hello', { a: 1 }]);
  });

  it('keeps engine wrap/unwrap/split steps on key paths out of the Studio forms', () => {
    expect(StepMapper.fromStep({ kind: 'wrap', field: 'key.id' })).toBeNull();
    expect(StepMapper.fromStep({ kind: 'split_array', field: 'value.items', to: 'key.item' })).toBeNull();
  });

  it('keeps a disabled step disabled', () => {
    const step = StepMapper.toStep(StepFactory.create(TransformStepKind.Hash, { field: 'a', enabled: false }));
    expect(step).toEqual({ kind: 'hash', field: 'value.a', enabled: false });
  });
});
