import {
  ProjectedState,
  SampleSummary,
  StepProjection,
  type EventResult,
  type ProjectedEvent,
  type ProjectionPoint,
} from '../../domain';
import type { SampleRecord } from '../../domain';
import { PreviewPhase, type PreviewState } from '../../store';
import { PanelMessages, type PanelSourceNames } from './PanelMessages';

/** One visible event of the panel: the record, its position and how it looks at the selected step. */
export type PanelRow = { record: SampleRecord; position: number; projected: ProjectedEvent };

/** View model of the sample panel: projected rows, search, pages, summary line and notices. */
export class SamplePanelModel {
  static readonly pageSize = 50;

  readonly rows: PanelRow[];
  readonly summary: string;
  readonly notices: string[];
  readonly loadError: string | null;
  readonly runError: string | null;
  readonly busy: string | null;

  constructor(state: PreviewState, point: ProjectionPoint, names: PanelSourceNames, reasonFor?: (position: number) => string | undefined) {
    const results = new Map<number, EventResult>((state.result?.results ?? []).map((r) => [r.seq, r]));
    this.rows = state.records
      .map((record, position) => SamplePanelModel.row(record, position, StepProjection.project(record, results.get(record.seq), point), reasonFor))
      .filter((row) => !state.searchHits || state.searchHits.has(row.record.seq));
    this.summary = state.result ? SampleSummary.label(state.result.summary) : SampleSummary.loadedLabel(state.records.length);
    this.notices = SamplePanelModel.noticesOf(state);
    this.loadError = state.loadFailure ? PanelMessages.loadFailure(state.loadFailure, names) : null;
    this.runError = state.runFailure ? PanelMessages.runFailure(state.runFailure) : null;
    this.busy = SamplePanelModel.busyOf(state);
  }

  pageCount(): number {
    return Math.max(1, Math.ceil(this.rows.length / SamplePanelModel.pageSize));
  }

  page(index: number): PanelRow[] {
    const start = Math.min(index, this.pageCount() - 1) * SamplePanelModel.pageSize;
    return this.rows.slice(start, start + SamplePanelModel.pageSize);
  }

  private static row(record: SampleRecord, position: number, projected: ProjectedEvent, reasonFor?: (position: number) => string | undefined): PanelRow {
    const reason = projected.reason ?? (projected.state === ProjectedState.Filtered ? reasonFor?.(position) : undefined);
    return { record, position, projected: { ...projected, reason } };
  }

  private static noticesOf(state: PreviewState): string[] {
    const notices: (string | null)[] = [
      state.session ? SampleSummary.demoNotice(state.request) : null,
      state.session ? SampleSummary.sessionNotice(state.session) : null,
      state.result ? SampleSummary.incompleteNotice(state.result.summary) : null,
    ];
    return notices.filter((n): n is string => !!n);
  }

  private static busyOf(state: PreviewState): string | null {
    if (state.loadPhase === PreviewPhase.Loading) return PanelMessages.loading;
    if (state.reformatting) return PanelMessages.reformatting;
    return state.runPhase === PreviewPhase.Loading ? PanelMessages.running : null;
  }
}
