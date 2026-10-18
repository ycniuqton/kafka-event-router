import { EventStatus, type EventResult, type JsonValue, type KafkaEvent, type RunResult, type SampleEvent, type TransformRunResult } from '../../../../domain';
import { PatchReplayer } from './PatchReplayer';
import type { StudioRunPlan } from './StudioRunPlan';
import { TraceEvent } from './TraceEvent';

/** One executed step of one sample: the event after it (null once dropped or failed), logs and error. */
export type StudioStepTrace = { event: KafkaEvent | null; logs: string[]; error: string | null };

/** Where and why a sample failed; `line`/`column` locate JS errors in the step's code. */
export type StudioEventError = { stepIndex: number | null; message: string; line: number | null; column: number | null };

/** The steps' result on one sample; `steps` is indexed by draft step index (missing = not executed). */
export type StudioEventRun = {
  sampleId: number;
  status: EventStatus;
  input: KafkaEvent;
  steps: (StudioStepTrace | undefined)[];
  output: KafkaEvent | null;
  outputBytes: number | null;
  droppedAt: number | null;
  error: StudioEventError | null;
};

/** Extracts the `(line:col)` the JS sandbox appends to script errors. */
export class ScriptErrorLocation {
  static readonly pattern = /\((\d+):(\d+)\)\s*$/;

  static parse(message: string): { line: number | null; column: number | null } {
    const match = ScriptErrorLocation.pattern.exec(message);
    return match ? { line: Number(match[1]), column: Number(match[2]) } : { line: null, column: null };
  }
}

/** Builds per-sample runs from a preview result, replaying step patches into snapshots. */
class EventRunBuilder {
  static build(plan: StudioRunPlan, sample: SampleEvent, result: EventResult | undefined): StudioEventRun {
    const input: KafkaEvent = { key: sample.key, headers: sample.headers, value: sample.value };
    const branch = result?.branches[0];
    const traces = branch?.steps ?? [];
    const lists = PatchReplayer.replay(TraceEvent.document(sample) as unknown as JsonValue, traces.map((t) => t.patch));
    const steps: (StudioStepTrace | undefined)[] = [];
    let droppedAt: number | null = null;
    traces.forEach((t, i) => {
      const index = plan.draftIndex(t.index);
      if (index === null) return;
      const event = lists[i].length ? TraceEvent.fromDocument(lists[i][0] as JsonValue, input) : null;
      if (!event && droppedAt === null && !t.error) droppedAt = index;
      steps[index] = { event: t.error ? null : event, logs: t.logs, error: t.error };
    });
    const status = result?.status ?? EventStatus.Error;
    const output = branch?.outputs[0];
    const lastIndex = plan.draftIndex(traces[traces.length - 1]?.index);
    return {
      sampleId: sample.id,
      status,
      input,
      steps,
      output: output ? TraceEvent.fromOutput(output) : null,
      outputBytes: output?.sizeBytes ?? null,
      droppedAt: status === EventStatus.Dropped ? (droppedAt ?? lastIndex) : null,
      error: EventRunBuilder.error(plan, result),
    };
  }

  private static error(plan: StudioRunPlan, result: EventResult | undefined): StudioEventError | null {
    const err = result?.error ?? result?.branches[0]?.error ?? null;
    if (!err) return null;
    return { stepIndex: plan.draftIndex(err.stepIndex), message: err.message, ...ScriptErrorLocation.parse(err.message) };
  }
}

/** A Studio run: every sample's result by sample id, plus step issues by draft step index. */
export class StudioRunResult {
  static readonly stepPointer = /^\/(?:config\/)?branches\/0\/transform\/steps\/(\d+)/;

  readonly runs: ReadonlyMap<number, StudioEventRun>;
  readonly issues: ReadonlyMap<number, string>;
  readonly durationMs: number;

  constructor(plan: StudioRunPlan, result: RunResult) {
    const bySeq = new Map(result.results.map((r) => [r.seq, r]));
    this.runs = new Map(plan.samples.map((s, i) => [s.id, EventRunBuilder.build(plan, s, bySeq.get(i))]));
    const issues = new Map(plan.skipped);
    for (const issue of result.issues) {
      const index = plan.draftIndex(StudioRunResult.runStepOf(issue.path));
      if (index !== null && !issues.has(index)) issues.set(index, issue.message);
    }
    this.issues = issues;
    this.durationMs = result.summary.durationMs;
  }

  /** Run step index addressed by a JSON Pointer into the run document, if any. */
  static runStepOf(path: string | undefined): number | null {
    const match = path ? StudioRunResult.stepPointer.exec(path) : null;
    return match ? Number(match[1]) : null;
  }

  run(sampleId: number | undefined): StudioEventRun | undefined {
    return sampleId === undefined ? undefined : this.runs.get(sampleId);
  }

  /** Result shown in Output: after all steps, or after the step `upto` (inclusive). */
  resultFor(sampleId: number, upto?: number): TransformRunResult | null {
    const run = this.runs.get(sampleId);
    if (!run) return null;
    const failedAt = run.error?.stepIndex ?? null;
    const logs = StudioRunResult.logs(run, upto);
    if (run.error && (upto === undefined || failedAt === null || failedAt <= upto)) {
      return { error: run.error.message, stepIndex: failedAt ?? undefined, event: this.eventBefore(run, failedAt ?? 0) ?? undefined, logs };
    }
    if (run.droppedAt !== null && (upto === undefined || run.droppedAt <= upto)) return { dropped: true, stepIndex: run.droppedAt, logs };
    if (upto === undefined) return { event: run.output ?? this.eventBefore(run, run.steps.length) ?? run.input, logs };
    return { event: this.eventBefore(run, upto + 1) ?? run.input, logs };
  }

  /** Event entering and leaving draft step `index` on one sample (card examples). */
  snapshots(sampleId: number | undefined, index: number): { before: KafkaEvent | null; after: KafkaEvent | null } {
    const run = this.run(sampleId);
    const trace = run?.steps[index];
    if (!run || !trace) return { before: null, after: null };
    return { before: this.eventBefore(run, index), after: trace.event };
  }

  /** The event entering draft step `index` (after the last executed step before it). */
  eventBefore(run: StudioEventRun, index: number): KafkaEvent | null {
    for (let i = Math.min(index, run.steps.length) - 1; i >= 0; i--) {
      const trace = run.steps[i];
      if (trace) return trace.event;
    }
    return run.input;
  }

  private static logs(run: StudioEventRun, upto?: number): string[] {
    return run.steps.slice(0, upto === undefined ? undefined : upto + 1).flatMap((t) => t?.logs ?? []);
  }
}
