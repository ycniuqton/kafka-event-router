import { DemoSamples } from './DemoSamples';
import { StopReason, type BranchSummary, type LoadSamplesRequest, type RunSummary, type SampleSession } from './types';

/** Summary line and notices of the sample panel (design §7 message patterns). */
export class SampleSummary {
  private static readonly numbers = new Intl.NumberFormat('en-US');

  static readonly emptyNotice = 'No events yet. Upload a JSON file to test.';

  /** "500 loaded · 37 pass · 461 filtered · 2 errors" (dropped only when present). */
  static label(summary: RunSummary): string {
    const n = (v: number) => SampleSummary.numbers.format(v);
    const parts = [`${n(summary.loaded)} loaded`, `${n(summary.passed)} pass`, `${n(summary.filtered)} filtered`, `${n(summary.errors)} errors`];
    if (summary.dropped > 0) parts.push(`${n(summary.dropped)} dropped`);
    return parts.join(' · ');
  }

  /** One branch's counts: "37 pass · 461 filtered · 2 errors" (dropped only when present). */
  static branchLabel(branch: BranchSummary): string {
    const n = (v: number) => SampleSummary.numbers.format(v);
    const parts = [`${n(branch.passed)} pass`, `${n(branch.filtered)} filtered`, `${n(branch.errors)} errors`];
    if (branch.dropped > 0) parts.push(`${n(branch.dropped)} dropped`);
    return parts.join(' · ');
  }

  /** Label before any run finished: only the loaded count. */
  static loadedLabel(loaded: number): string {
    return `${SampleSummary.numbers.format(loaded)} loaded`;
  }

  /** "Previewed 640 of 1,000 events (time limit)." when the run deadline hit, else null. */
  static incompleteNotice(summary: RunSummary): string | null {
    if (!summary.incomplete) return null;
    const n = SampleSummary.numbers;
    return `Previewed ${n.format(summary.processed)} of ${n.format(summary.loaded)} events (time limit).`;
  }

  /** Tells that the samples are synthetic demo events (demo source), else null. */
  static demoNotice(request: LoadSamplesRequest | null): string | null {
    return DemoSamples.isDemo(request) ? DemoSamples.notice : null;
  }

  /** Why a load stopped early ("Showing first 1,000 events (limit).") or that it is empty. */
  static sessionNotice(session: SampleSession): string | null {
    if (session.loaded === 0) return SampleSummary.emptyNotice;
    if (!session.truncated) return null;
    const count = SampleSummary.numbers.format(session.loaded);
    return session.stopReason === StopReason.Deadline ? `Showing first ${count} events (time limit).` : `Showing first ${count} events (limit).`;
  }
}
