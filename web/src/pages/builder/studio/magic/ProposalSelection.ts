import type { StepProposal } from '../../../../api';
import type { TransformStep } from '../../../../domain';

/** Texts of proposal diffs (AI assist and transform by example). */
export class ProposalText {
  static readonly accept = 'Accept';
  static readonly acceptSelected = (n: number) => `Accept ${n} step${n === 1 ? '' : 's'}`;
  static readonly reject = 'Reject';
  static readonly lowConfidence = 'Low confidence — check this step before accepting.';
  static readonly blocked = 'Cannot be accepted: ';
  static readonly none = 'No recipe matched. Try "mask email", "rename a to b", "remove items".';
  static readonly addedByExample = 'Steps proposed from an example';

  static confidence(value: number): string {
    return `${Math.round(value * 100)}%`;
  }
}

/**
 * Which proposed steps the user keeps: every valid proposal starts selected; proposals with a blocking
 * issue can never be accepted; confidence below `lowConfidence` is highlighted. Nothing is applied until accept().
 */
export class ProposalSelection {
  static readonly lowConfidence = 0.6;
  readonly proposals: StepProposal[];
  private readonly selected: ReadonlySet<number>;

  constructor(proposals: StepProposal[], selected?: ReadonlySet<number>) {
    this.proposals = proposals;
    this.selected = selected ?? new Set(proposals.flatMap((p, i) => (p.issue ? [] : [i])));
  }

  static isLow(p: StepProposal): boolean {
    return p.confidence !== undefined && p.confidence < ProposalSelection.lowConfidence;
  }

  isSelected(index: number): boolean {
    return this.selected.has(index);
  }

  canSelect(index: number): boolean {
    return !this.proposals[index]?.issue;
  }

  toggle(index: number): ProposalSelection {
    if (!this.canSelect(index)) return this;
    const next = new Set(this.selected);
    if (next.has(index)) next.delete(index);
    else next.add(index);
    return new ProposalSelection(this.proposals, next);
  }

  get count(): number {
    return this.selected.size;
  }

  /** Steps to append, in proposal order. */
  accepted(): TransformStep[] {
    return this.proposals.filter((p, i) => this.selected.has(i) && !p.issue).map((p) => p.step);
  }
}
