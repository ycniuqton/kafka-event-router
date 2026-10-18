import { describe, expect, it } from 'vitest';
import { TransformRunner, TransformStepKind } from '../../domain';
import { MockMappingInferrer } from './MockMappingInferrer';

describe('MockMappingInferrer', () => {
  it('proposes renames, moves, constants and removals that produce the example', () => {
    const input = { cust_id: 'c-1', customer: { email: 'a@x' }, debug: true };
    const desired = { customerId: 'c-1', contact: { email: 'a@x' }, source: 'web' };
    const res = MockMappingInferrer.infer(input, desired);
    expect(res.proposals.map((p) => p.step.kind)).toEqual([TransformStepKind.Rename, TransformStepKind.Move, TransformStepKind.Set, TransformStepKind.Remove]);
    expect(res.mappings.find((m) => m.target === 'source')?.confidence).toBe(0.5);
    const out = TransformRunner.run({ key: '', headers: {}, value: structuredClone(input) }, res.proposals.map((p) => p.step));
    expect(out.event?.value).toEqual(desired);
  });
});
