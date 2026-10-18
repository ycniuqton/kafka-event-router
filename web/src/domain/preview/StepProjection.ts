import { BuilderStep } from '../builder';
import type { JsonValue } from '../json';
import { EventDocuments, type EventDocument } from './EventDocument';
import { BranchResults } from './BranchResults';
import { JsonPatchApplier } from './JsonPatchApplier';
import { EventStatus, type BranchResult, type EventResult, type PreviewOutput, type SampleRecord } from './types';

/** What the panel shows for one event at one builder step. */
export const ProjectedState = { Loaded: 'loaded', ...EventStatus } as const;
export type ProjectedState = (typeof ProjectedState)[keyof typeof ProjectedState];

export type ProjectedEvent = {
  state: ProjectedState;
  document: EventDocument;
  /** True when `document` differs from the record as read (after a transform step). */
  changed: boolean;
  reason?: string;
  outputs?: PreviewOutput[];
};

/** Where in the pipeline the panel looks: a builder step, for transforms the last step shown, and the branch (first when unset). */
export type ProjectionPoint = { step: BuilderStep; stepIndex?: number; branch?: string | null };

/** Projects a run result onto the selected builder step (design §5 "User selects a builder step"). */
export class StepProjection {
  static readonly droppedReason = 'Dropped by a step';

  static project(record: SampleRecord, eventResult: EventResult | undefined, point: ProjectionPoint): ProjectedEvent {
    const input = EventDocuments.of(record);
    if (point.step === BuilderStep.Source || !eventResult) return StepProjection.loaded(record, input);
    const { result, branch } = BranchResults.view(eventResult, point.branch);
    if (record.decodeError) return { state: ProjectedState.Error, document: input, changed: false, reason: record.decodeError };
    if (point.step === BuilderStep.Filter) return StepProjection.afterFilter(result, input);
    if (result.status === EventStatus.Filtered) return StepProjection.afterFilter(result, input);
    return StepProjection.afterSteps(result, branch, input, point.stepIndex);
  }

  /** The event after applying the patches of steps 0..uptoIndex (all steps when omitted). */
  static documentAfter(input: EventDocument, branch: BranchResult | undefined, uptoIndex?: number): EventDocument {
    const steps = (branch?.steps ?? []).filter((s) => uptoIndex === undefined || s.index <= uptoIndex);
    if (!steps.length) return input;
    try {
      const patched = JsonPatchApplier.applyAll(input as unknown as JsonValue, steps.map((s) => s.patch));
      return EventDocuments.asDocument(patched, input);
    } catch {
      return input;
    }
  }

  private static loaded(record: SampleRecord, input: EventDocument): ProjectedEvent {
    const reason = record.decodeError ?? undefined;
    return { state: reason ? ProjectedState.Error : ProjectedState.Loaded, document: input, changed: false, reason };
  }

  private static afterFilter(result: EventResult, input: EventDocument): ProjectedEvent {
    if (result.status === EventStatus.Filtered) return { state: ProjectedState.Filtered, document: input, changed: false };
    const filterError = result.error && result.error.stepIndex === null ? result.error.message : undefined;
    if (filterError) return { state: ProjectedState.Error, document: input, changed: false, reason: filterError };
    return { state: ProjectedState.Passed, document: input, changed: false };
  }

  private static afterSteps(result: EventResult, branch: BranchResult | undefined, input: EventDocument, uptoIndex?: number): ProjectedEvent {
    const document = StepProjection.documentAfter(input, branch, uptoIndex);
    const changed = document !== input;
    const failedAt = result.error?.stepIndex ?? null;
    const reached = uptoIndex === undefined || failedAt === null || failedAt <= uptoIndex;
    if (result.status === EventStatus.Error && reached) {
      return { state: ProjectedState.Error, document, changed, reason: StepProjection.errorText(result) };
    }
    if (result.status === EventStatus.Dropped && reached) {
      return { state: ProjectedState.Dropped, document, changed, reason: StepProjection.droppedReason };
    }
    return { state: ProjectedState.Passed, document, changed, outputs: branch?.outputs };
  }

  private static errorText(result: EventResult): string {
    const err = result.error;
    if (!err) return '';
    return err.step ? `${err.step}: ${err.message}` : err.message;
  }
}
