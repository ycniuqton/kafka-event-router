import { describe, expect, it } from 'vitest';
import type { Step } from '../../../../api/generated';
import {
  JsonFlattener,
  MetadataName,
  TransformRunner,
  TransformStepKind,
  ValueSourceKind,
  type KafkaEvent,
  type TransformStep,
} from '../../../../domain';
import { StepMapper } from '../../../../domain/routeConfig/StepMapper';
import { StepCompleteness } from '../preview/StepCompleteness';
import { RecipeFormSpec, RecipeInput } from './RecipeFormSpec';
import { StepMenuModel } from './StepMenuModel';
import { StepPresenters } from './StepPresenters';

/** Text typed into each input of a form (keyed by input), as a user fills it in the Studio. */
type Typed = Partial<Record<RecipeInput, string>>;

/** Fills a structure / header recipe form through its spec, starting from the "+ Add step" defaults. */
class FormFill {
  static readonly sample: KafkaEvent = {
    key: 'o-1',
    headers: { traceId: 't-9', 'x-internal': '1' },
    value: { orderId: 'o-1', items: [{ sku: 'A', qty: 1 }, { sku: 'B', qty: 2 }], data: { order: { id: 7 } } },
  };

  static step(kind: TransformStepKind, typed: Typed, source?: Partial<TransformStep>): TransformStep {
    let step = StepMenuModel.create(kind, JsonFlattener.flatten(FormFill.sample.value));
    for (const { input } of RecipeFormSpec.of(kind).inputs) {
      if (input === RecipeInput.Source) step = { ...step, ...source };
      else if (typed[input] !== undefined) step = { ...step, ...RecipeFormSpec.patch(input, typed[input] as string) };
    }
    return step;
  }

  static run(step: TransformStep): KafkaEvent | undefined {
    return TransformRunner.run(FormFill.sample, [step]).event;
  }
}

type Case = { kind: TransformStepKind; typed: Typed; source?: Partial<TransformStep>; engine: Step };

const cases: Case[] = [
  { kind: TransformStepKind.SplitArray, typed: { target: 'item' }, engine: { kind: 'split_array', field: 'value.items', to: 'value.item' } },
  { kind: TransformStepKind.SplitArray, typed: {}, engine: { kind: 'split_array', field: 'value.items' } },
  { kind: TransformStepKind.Wrap, typed: { fieldPath: 'payload' }, engine: { kind: 'wrap', field: 'value.payload' } },
  { kind: TransformStepKind.Unwrap, typed: { fieldPick: 'data.order' }, engine: { kind: 'unwrap', field: 'value.data.order' } },
  {
    kind: TransformStepKind.HeaderAdd,
    typed: { header: 'source' },
    source: { sourceKind: ValueSourceKind.Metadata, argument: MetadataName.Topic },
    engine: { kind: 'header_add', header: 'source', source: { from: 'metadata', name: 'topic' } },
  },
  { kind: TransformStepKind.HeaderAdd, typed: { header: 'v' }, source: { argument: '2' }, engine: { kind: 'header_add', header: 'v', source: { from: 'constant', value: 2 } } },
  { kind: TransformStepKind.HeaderRemove, typed: { header: 'x-internal' }, engine: { kind: 'header_remove', header: 'x-internal' } },
  { kind: TransformStepKind.HeaderRename, typed: { header: 'traceId', newHeader: 'trace-id' }, engine: { kind: 'header_rename', header: 'traceId', newHeader: 'trace-id' } },
  { kind: TransformStepKind.HeaderToPayload, typed: { header: 'traceId', fieldPath: 'meta.traceId' }, engine: { kind: 'header_copy_to_payload', header: 'traceId', field: 'value.meta.traceId' } },
  { kind: TransformStepKind.HeaderFromPayload, typed: { fieldPick: 'orderId', header: 'order-id' }, engine: { kind: 'header_copy_from_payload', header: 'order-id', field: 'value.orderId' } },
];

describe('structure + Kafka header recipe forms', () => {
  it.each(cases)('$kind form emits $engine.kind with the engine option names', ({ kind, typed, source, engine }) => {
    const step = FormFill.step(kind, typed, source);
    expect(StepCompleteness.reason(step)).toBeNull();
    expect(StepMapper.toStep(step)).toEqual(engine);
    expect(StepMapper.toStep(StepMapper.fromStep(engine) as TransformStep)).toEqual(engine);
  });

  it('is listed in the Structure and Kafka groups of the step menu', () => {
    expect(StepMenuModel.search('split').flatMap((g) => g.items.map((i) => i.kind))).toEqual([TransformStepKind.SplitArray]);
    expect(StepMenuModel.search('structure')[0].items.map((i) => i.kind)).toEqual([TransformStepKind.SplitArray, TransformStepKind.Wrap, TransformStepKind.Unwrap]);
    expect(StepMenuModel.search('header').flatMap((g) => g.items).length).toBe(5);
  });

  it('asks for the header name before a header step can run', () => {
    expect(StepCompleteness.reason(FormFill.step(TransformStepKind.HeaderRemove, {}))).toBe('Enter the header name.');
    expect(StepCompleteness.reason(FormFill.step(TransformStepKind.HeaderRename, { header: 'a' }))).toBe('Enter the new header name.');
    expect(StepCompleteness.reason(FormFill.step(TransformStepKind.Wrap, {}))).toBe('Choose a field.');
  });

  it('shows before → after examples of the header or field each step changes', () => {
    const rename = FormFill.step(TransformStepKind.HeaderRename, { header: 'traceId', newHeader: 'trace-id' });
    const example = (s: TransformStep) => StepPresenters.of(s.kind).example(s, { before: FormFill.sample, after: FormFill.run(s) ?? null });
    expect(example(rename)).toEqual({ before: 'traceId: t-9', after: 'trace-id: t-9' });
    expect(example(FormFill.step(TransformStepKind.HeaderRemove, { header: 'x-internal' }))).toEqual({ before: 'x-internal: 1', after: 'x-internal (no header)' });
    expect(example(FormFill.step(TransformStepKind.HeaderToPayload, { header: 'traceId', fieldPath: 'traceId' }))).toEqual({ before: 'traceId: t-9', after: 'traceId: t-9' });
    expect(example(FormFill.step(TransformStepKind.SplitArray, { target: 'item' }))).toEqual({ before: 'items: 2 items', after: 'item: {"sku":"A","qty":1}' });
    expect(example(FormFill.step(TransformStepKind.Unwrap, { fieldPick: 'data.order' }))).toEqual({ before: 'data.order: {"id":7}', after: 'value: {"id":7}' });
  });

  it('summarises each step on its card', () => {
    const summary = (kind: TransformStepKind, typed: Typed, source?: Partial<TransformStep>) => StepPresenters.of(kind).summary(FormFill.step(kind, typed, source));
    expect(summary(TransformStepKind.Wrap, { fieldPath: 'payload' })).toBe('value → payload');
    expect(summary(TransformStepKind.HeaderAdd, { header: 'src' }, { sourceKind: ValueSourceKind.Field, argument: 'orderId' })).toBe('src ← field orderId');
    expect(summary(TransformStepKind.HeaderFromPayload, { fieldPick: 'orderId', header: 'oid' })).toBe('orderId → oid');
  });

  it('runs in the in-browser runner like the engine (wrap, header copy, split keeps the first item)', () => {
    expect(FormFill.run(FormFill.step(TransformStepKind.Wrap, { fieldPath: 'payload' }))?.value).toEqual({ payload: FormFill.sample.value });
    expect(FormFill.run(FormFill.step(TransformStepKind.HeaderFromPayload, { fieldPick: 'orderId', header: 'oid' }))?.headers.oid).toBe('o-1');
    expect(FormFill.run(FormFill.step(TransformStepKind.SplitArray, {}))?.value.items).toEqual({ sku: 'A', qty: 1 });
  });
});
