import { describe, expect, it } from 'vitest';
import type { JsonValue } from '../json';
import { JsonPatchApplier } from './JsonPatchApplier';
import { JsonPatchDiffer } from './JsonPatchDiffer';
import type { PatchOp } from './types';
import casesText from '../../../../internal/preview/testdata/patch/cases.json?raw';

/** Golden cases shared with the Go `preview.StructuralDiffer` (internal/preview/testdata/patch/cases.json). */
type PatchCase = { name: string; before: JsonValue; after: JsonValue; patch: PatchOp[] };

const cases: PatchCase[] = JSON.parse(casesText);

describe('JsonPatchDiffer / JsonPatchApplier (Go ↔ TS fixtures)', () => {
  it('loads the shared fixtures', () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  it.each(cases.map((c) => [c.name, c] as const))('%s: diff matches the Go patch', (_, c) => {
    expect(JsonPatchDiffer.diff(c.before, c.after)).toEqual(c.patch);
  });

  it.each(cases.map((c) => [c.name, c] as const))('%s: applying the patch gives the after document', (_, c) => {
    expect(JsonPatchApplier.apply(c.before, c.patch)).toEqual(c.after);
  });

  it('does not mutate the input document', () => {
    const before = { value: { a: 1 } };
    JsonPatchApplier.apply(before, [{ op: 'replace', path: '/value/a', value: 2 }]);
    expect(before).toEqual({ value: { a: 1 } });
  });

  it('applies consecutive step patches in order', () => {
    const doc = JsonPatchApplier.applyAll({ value: { cust_id: 1 } }, [
      [{ op: 'remove', path: '/value/cust_id' }, { op: 'add', path: '/value/customerId', value: 1 }],
      [{ op: 'replace', path: '/value/customerId', value: 2 }],
    ]);
    expect(doc).toEqual({ value: { customerId: 2 } });
  });
});
