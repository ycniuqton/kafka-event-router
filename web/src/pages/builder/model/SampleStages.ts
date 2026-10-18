import { BuilderStep, FieldPath, FilterMode, FilterTrace, type ProjectionPoint, type RouteDraft } from '../../../domain';
import type { PipelinePreviewResult } from './PipelinePreview';

export type StageContext = { draft: RouteDraft; preview: PipelinePreviewResult };

/** How the sample panel shows events at one builder step: header label, tip and projection point. */
export abstract class SampleStage {
  static readonly keepOnlyTip = 'Click any value to keep only events matching it.';

  abstract readonly label: string;
  abstract readonly showTip: boolean;
  abstract readonly step: BuilderStep;

  point(): ProjectionPoint {
    return { step: this.step };
  }

  /** Reason line of a filtered sample naming the first condition that failed (design `stepRes`). */
  static filteredReason(index: number, { draft, preview }: StageContext): string | undefined {
    const result = preview.filterResults[index];
    if (!result || result.pass) return undefined;
    if (draft.filter.mode === FilterMode.Expression) return 'Expression returned false';
    if (draft.filter.mode === FilterMode.Code) return 'Filter function returned false';
    const failed = FilterTrace.firstFailed(result.rowResults);
    const row = draft.filter.rows[failed] ?? draft.filter.rows[0];
    return row ? `${FieldPath.short(row.field)} ${row.op} ${row.value} → false` : undefined;
  }
}

class FixedStage extends SampleStage {
  readonly label: string;
  readonly showTip: boolean;
  readonly step: BuilderStep;

  constructor(step: BuilderStep, label: string, showTip: boolean) {
    super();
    this.step = step;
    this.label = label;
    this.showTip = showTip;
  }
}

/** Stage used for each builder step. */
export class SampleStageRegistry {
  private static readonly stages: Record<BuilderStep, SampleStage> = {
    [BuilderStep.Source]: new FixedStage(BuilderStep.Source, 'as read', true),
    [BuilderStep.Filter]: new FixedStage(BuilderStep.Filter, 'after Filter', true),
    [BuilderStep.Transform]: new FixedStage(BuilderStep.Transform, 'after Transform', false),
    [BuilderStep.Destination]: new FixedStage(BuilderStep.Destination, 'final output', false),
    [BuilderStep.Review]: new FixedStage(BuilderStep.Review, 'final output', false),
  };

  static for(step: BuilderStep): SampleStage {
    return SampleStageRegistry.stages[step];
  }
}
