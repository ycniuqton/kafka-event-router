import type { BranchResult, BranchSummary, EventResult, RunResult } from './types';

/** An event result seen through one branch: that branch's status, error and trace. */
export type BranchView = { result: EventResult; branch: BranchResult | undefined };

/** Per-branch reading of run results (FR-14): branch ids, a branch's counts and an event seen through a branch. */
export class BranchResults {
  /** Branch ids of a run in document order (empty before the first run). */
  static ids(result: RunResult | null): string[] {
    return (result?.summary.branches ?? []).map((b) => b.branch);
  }

  /** Counts of one branch, if the run has it. */
  static summary(result: RunResult | null, branch: string | null | undefined): BranchSummary | undefined {
    return branch ? result?.summary.branches?.find((b) => b.branch === branch) : undefined;
  }

  /** The event through `branch`; without a branch (or for events without branch traces) the event-level view. */
  static view(result: EventResult, branch?: string | null): BranchView {
    const selected = branch ? result.branches.find((b) => b.branch === branch) : undefined;
    if (!selected) return { result, branch: result.branches[0] };
    return { result: { ...result, status: selected.status, error: selected.error ?? result.error, branches: [selected] }, branch: selected };
  }
}
