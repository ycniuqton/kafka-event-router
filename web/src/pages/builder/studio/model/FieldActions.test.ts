import { describe, expect, it } from 'vitest';
import { JsonFlattener, MaskMode, TransformStepKind, type JsonLine } from '../../../../domain';
import { FieldActionCatalog } from './FieldActions';
import { StepListEditor } from './StepListEditor';

/** Lines of a small sample payload by path. */
class TestLines {
  static readonly lines = JsonFlattener.flatten({ email: 'ada@example.com', card: '4111111111111111', cust_id: 'c-1', orderId: 'o-9' });

  static at(path: string): JsonLine {
    return TestLines.lines.find((l) => l.path === path) as JsonLine;
  }
}

describe('Click-to-transform (ClickActionRegistry)', () => {
  const byId = new Map(FieldActionCatalog.all.map((a) => [a.id, a]));

  it('offers Mask · Rename · Remove · Use as key in that order', () => {
    expect(FieldActionCatalog.all.map((a) => a.label)).toEqual(['Mask', 'Rename', 'Remove', 'Use as key']);
  });

  it('guesses the mask style from the value shape', () => {
    expect(byId.get('mask')?.build(TestLines.at('email'))).toMatchObject({ kind: TransformStepKind.Mask, field: 'email', maskMode: MaskMode.Email });
    expect(byId.get('mask')?.build(TestLines.at('card'))).toMatchObject({ maskMode: MaskMode.Last4 });
  });

  it('proposes a camelCase name, removes and keys by the clicked path', () => {
    expect(byId.get('rename')?.build(TestLines.at('cust_id'))).toMatchObject({ kind: TransformStepKind.Rename, field: 'cust_id', argument: 'custId' });
    expect(byId.get('remove')?.build(TestLines.at('card'))).toMatchObject({ kind: TransformStepKind.Remove, field: 'card' });
    expect(byId.get('key')?.build(TestLines.at('orderId'))).toMatchObject({ kind: TransformStepKind.SetKey, field: 'orderId' });
  });

  it('inserts the new step after the selected step', () => {
    const mask = byId.get('mask')?.build(TestLines.at('email'));
    const hash = byId.get('remove')?.build(TestLines.at('card'));
    const key = byId.get('key')?.build(TestLines.at('orderId'));
    if (!mask || !hash || !key) throw new Error('missing action');
    const edit = new StepListEditor([mask, hash]).insertAfter(0, [key], 'Added');
    expect(edit.steps.map((s) => s.kind)).toEqual([TransformStepKind.Mask, TransformStepKind.SetKey, TransformStepKind.Remove]);
  });
});
