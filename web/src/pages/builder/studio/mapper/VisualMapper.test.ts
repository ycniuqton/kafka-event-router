import { describe, expect, it } from 'vitest';
import { MapEntryEditor, MappedFields, MapStart, StepFactory, TransformRunner, TransformStepKind, ValueSourceKind, type MapEntry } from '../../../../domain';
import { MapTreeCodec } from '../../../../domain/routeConfig/MapTreeCodec';
import { StepMapper } from '../../../../domain/routeConfig/StepMapper';
import { FormulaCompletion } from './FormulaCompletion';
import { OutputTreeModel, OutputRowType } from './OutputTreeModel';

const entries: MapEntry[] = [
  { path: 'customer.id', sourceKind: ValueSourceKind.Field, argument: 'cust_id' },
  { path: 'customer.mail', sourceKind: ValueSourceKind.Field, argument: 'email' },
  { path: 'cust_id', sourceKind: ValueSourceKind.Constant, argument: '', remove: true },
  { path: 'total', sourceKind: ValueSourceKind.Formula, argument: 'value.amount * 2' },
  { path: 'source', sourceKind: ValueSourceKind.Metadata, argument: 'topic' },
];

describe('Visual mapper serialisation', () => {
  it('round-trips entries through the engine map step', () => {
    const step = StepFactory.create(TransformStepKind.Map, { mapStart: MapStart.Empty, mapEntries: entries });
    const doc = StepMapper.toStep(step);
    expect(doc.start).toBe('empty');
    expect(doc.target?.fields?.customer?.fields?.id?.source).toEqual({ from: 'field', path: 'value.cust_id' });
    expect(doc.target?.fields?.cust_id).toEqual({ remove: true });
    const back = StepMapper.fromStep(doc);
    expect(back?.kind).toBe(TransformStepKind.Map);
    expect(new Set(back?.mapEntries?.map((e) => e.path))).toEqual(new Set(entries.map((e) => e.path)));
    expect(StepMapper.toStep(back!)).toEqual(doc);
  });

  it('refuses a tree with a source the mapper cannot show', () => {
    expect(MapTreeCodec.fromTarget({ fields: { a: { source: { from: 'nope' as never } } } })).toBeNull();
  });

  it('edits entries: map, rename, nest, un-nest, remove input', () => {
    let list = new MapEntryEditor([]).mapField('cust_id');
    expect(list).toEqual([{ path: 'cust_id', sourceKind: ValueSourceKind.Field, argument: 'cust_id' }]);
    list = new MapEntryEditor(list).mapField('cust_id');
    expect(list[1].path).toBe('cust_id_2');
    list = new MapEntryEditor(list).rename(1, 'customerId');
    list = new MapEntryEditor(list).nest(1, 'customer');
    expect(list[1].path).toBe('customer.customerId');
    list = new MapEntryEditor(list).unnest(1);
    expect(list[1].path).toBe('customerId');
    list = new MapEntryEditor(list).removeInput('email');
    expect(list[2]).toMatchObject({ path: 'email', remove: true });
  });

  it('lists unmapped input leaves for copy-input and empty starts', () => {
    const input = { cust_id: 'c1', email: 'a@b', amount: 3, items: { n: 1 } };
    expect(MappedFields.unmapped(input, MapStart.CopyInput, entries)).toEqual([]);
    expect(MappedFields.unmapped(input, MapStart.CopyInput, new MapEntryEditor(entries).removeInput('amount'))).toEqual(['amount']);
    expect(MappedFields.unmapped(input, MapStart.Empty, entries)).toEqual(['amount', 'items.n']);
  });

  it('applies the mapping in the mock runner like the engine (sources from the input, removals first)', () => {
    const step = StepFactory.create(TransformStepKind.Map, { mapStart: MapStart.CopyInput, mapEntries: entries.slice(0, 3) });
    const out = TransformRunner.run({ key: '', headers: {}, value: { cust_id: 'c1', email: 'a@b' } }, [step]);
    expect(out.event?.value).toEqual({ email: 'a@b', customer: { id: 'c1', mail: 'a@b' } });
  });

  it('builds output tree rows with object groups', () => {
    const rows = OutputTreeModel.rows(entries);
    expect(rows.map((r) => `${r.type}:${r.path}`)).toEqual([
      `${OutputRowType.Entry}:cust_id`,
      `${OutputRowType.Group}:customer`,
      `${OutputRowType.Entry}:customer.id`,
      `${OutputRowType.Entry}:customer.mail`,
      `${OutputRowType.Entry}:source`,
      `${OutputRowType.Entry}:total`,
    ]);
  });
});

describe('FormulaCompletion', () => {
  const fns = [{ name: 'concat', call: 'concat(...values)', description: 'Joins' }, { name: 'coalesce', call: 'coalesce(a, b)', description: 'First' }];
  it('suggests functions and value fields for the token at the caret', () => {
    const text = 'upper(value.em';
    const res = FormulaCompletion.suggest(text, text.length, ['email', 'amount'], fns);
    expect(res.items.map((i) => i.insert)).toEqual(['value.email']);
    const applied = FormulaCompletion.apply(text, res, 'value.email');
    expect(applied).toEqual({ text: 'upper(value.email', caret: 17 });
    expect(FormulaCompletion.suggest('co', 2, [], fns).items.map((i) => i.insert)).toEqual(['concat(', 'coalesce(']);
    expect(FormulaCompletion.suggest('a + ', 4, ['a'], fns).items).toEqual([]);
  });
});
