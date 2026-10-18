import type { SampleEvent } from '../../../../domain';
import type { StudioRunResult } from '../preview/StudioRunResult';

/** ctx.log output of one sample at a step. */
export type ConsoleEntry = { sampleId: number; lines: string[]; error?: string };

/** Collects ctx.log() output per sample event of one JS step from the preview trace (Console tab). */
export class StepConsole {
  static collect(samples: SampleEvent[], run: StudioRunResult | null, stepIndex: number): ConsoleEntry[] {
    if (!run) return [];
    return samples
      .map((s) => {
        const trace = run.run(s.id)?.steps[stepIndex];
        return { sampleId: s.id, lines: trace?.logs ?? [], error: trace?.error ?? undefined };
      })
      .filter((e) => e.lines.length > 0 || e.error !== undefined);
  }
}
