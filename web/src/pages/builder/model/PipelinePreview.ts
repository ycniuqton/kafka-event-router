import {
  BranchResults,
  BranchSet,
  EventDocuments,
  EventStatus,
  FilterMode,
  FilterTrace,
  SampleRecords,
  StepProjection,
  type EventResult,
  type KafkaEvent,
  type RouteDraft,
  type RowResult,
  type RunResult,
  type SampleRecord,
  type TransformRunResult,
} from '../../../domain';

/** Filter outcome of one sample as traced by the preview service; rows are null when not evaluated. */
export type StageFilterResult = { pass: boolean; rowResults: RowResult[]; error?: string };

/** Filter and transform outcome of every loaded sample, aligned by sample position. */
export type PipelinePreviewResult = {
  /** False until the preview service answered for the current draft. */
  ready: boolean;
  total: number;
  filterResults: StageFilterResult[];
  outputs: (TransformRunResult | null)[];
  passIndexes: number[];
  errorIndexes: number[];
};

/** Projects the preview service's run result (production engine) onto the builder's per-sample view. */
export class PipelinePreview {
  static readonly percent = 100;

  /** Results seen through the edited branch (P2 branches: its own status and trace). */
  static fromRun(records: SampleRecord[], run: RunResult | null, draft: RouteDraft): PipelinePreviewResult {
    const branch = BranchSet.at(draft);
    const bySeq = new Map((run?.results ?? []).map((r) => [r.seq, BranchResults.view(r, branch.id).result]));
    const rowCount = draft.filter.mode === FilterMode.Visual ? draft.filter.rows.length : 0;
    const results = records.map((r) => bySeq.get(r.seq));
    const filterResults = results.map((r) => PipelinePreview.filterOf(r, rowCount, FilterTrace.filterOf(branch.index)));
    const outputs = records.map((rec, i) => PipelinePreview.outputOf(rec, results[i]));
    const passIndexes = filterResults.map((_, i) => i).filter((i) => filterResults[i].pass);
    const errorIndexes = passIndexes.filter((i) => !!outputs[i]?.error);
    return { ready: !!run, total: records.length, filterResults, outputs, passIndexes, errorIndexes };
  }

  static matchPercent(result: PipelinePreviewResult): number {
    return result.total ? (result.passIndexes.length / result.total) * PipelinePreview.percent : 0;
  }

  /** Samples handed to the Transform Studio: those that pass the filter, or all when none do. */
  static studioSamples<T>(samples: T[], result: PipelinePreviewResult): T[] {
    return result.passIndexes.length ? result.passIndexes.map((i) => samples[i]) : samples;
  }

  private static filterOf(result: EventResult | undefined, rowCount: number, filter: string): StageFilterResult {
    if (!result) return { pass: true, rowResults: [] };
    const branch = result.branches[0];
    const filterError = result.status === EventStatus.Error && result.error?.stepIndex == null ? result.error?.message : undefined;
    const pass = result.status !== EventStatus.Filtered && !filterError;
    return { pass, rowResults: FilterTrace.rowResults(branch, rowCount, filter), error: filterError };
  }

  private static outputOf(record: SampleRecord, result: EventResult | undefined): TransformRunResult | null {
    if (!result || result.status === EventStatus.Filtered) return null;
    const branch = result.branches[0];
    const logs = (branch?.steps ?? []).flatMap((s) => s.logs);
    const doc = StepProjection.documentAfter(EventDocuments.of(record), branch);
    const event = PipelinePreview.event(record, doc.value, doc.key, doc.headers);
    const stepIndex = result.error?.stepIndex ?? undefined;
    if (result.status === EventStatus.Error) return { error: result.error?.message ?? '', stepIndex, event, logs };
    if (result.status === EventStatus.Dropped) return { dropped: true, stepIndex, logs };
    return { event, logs };
  }

  private static event(record: SampleRecord, value: SampleRecord['value'], key: SampleRecord['key'], headers: SampleRecord['headers']): KafkaEvent {
    const sample = SampleRecords.toEvent({ ...record, value, key, headers });
    return { key: sample.key, headers: sample.headers, value: sample.value };
  }
}
