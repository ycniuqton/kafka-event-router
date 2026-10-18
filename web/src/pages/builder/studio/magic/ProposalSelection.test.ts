import { describe, expect, it } from 'vitest';
import { StepFactory, TransformStepKind } from '../../../../domain';
import type { StepProposal } from '../../../../api';
import { ProposalSelection } from './ProposalSelection';

const proposal = (field: string, extra: Partial<StepProposal> = {}): StepProposal => ({
  text: field,
  step: StepFactory.create(TransformStepKind.Hash, { field }),
  ...extra,
});

describe('ProposalSelection', () => {
  it('selects valid proposals, never blocked ones, and accepts in order', () => {
    const sel = new ProposalSelection([proposal('a'), proposal('b', { issue: 'bad path' }), proposal('c')]);
    expect(sel.count).toBe(2);
    expect(sel.canSelect(1)).toBe(false);
    expect(sel.toggle(1)).toBe(sel);
    expect(sel.accepted().map((s) => s.field)).toEqual(['a', 'c']);
    const fewer = sel.toggle(0);
    expect(fewer.accepted().map((s) => s.field)).toEqual(['c']);
    expect(fewer.toggle(0).count).toBe(2);
  });

  it('flags confidence below 0.6 and ignores proposals without one', () => {
    expect(ProposalSelection.isLow(proposal('a', { confidence: 0.4 }))).toBe(true);
    expect(ProposalSelection.isLow(proposal('a', { confidence: 0.6 }))).toBe(false);
    expect(ProposalSelection.isLow(proposal('a'))).toBe(false);
  });
});
