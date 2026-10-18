import { useCallback, useMemo } from 'react';
import { SamplePanel } from '../../../components/sample-panel';
import { BuilderStep, FilterMode, PanelValueActionKind } from '../../../domain';
import type { BuilderStepContext } from '../model/BuilderStepContext';
import { SampleStage, SampleStageRegistry } from '../model/SampleStages';

/** Value actions the builder's panel offers: quick filter only (studio actions live in the Transform Studio). */
class BuilderValueActions {
  static readonly kinds = [PanelValueActionKind.KeepOnly] as const;
}

/** The builder's right zone: live samples after the selected step, quick filter and "Why?". */
export function BuilderSamplePanel({ ctx }: { ctx: BuilderStepContext }) {
  const { draft, step, live, preview } = ctx;
  const stage = SampleStageRegistry.for(step);
  const point = useMemo(() => stage.point(), [stage]);
  const reasonFor = useCallback((index: number) => SampleStage.filteredReason(index, { draft, preview }), [draft, preview]);
  const canWhy = step === BuilderStep.Filter && draft.filter.mode === FilterMode.Visual && draft.filter.rows.length > 0;
  const why = (seq: number) => {
    const index = live.state.records.findIndex((r) => r.seq === seq);
    ctx.setWhyIndex(ctx.whyIndex === index ? null : index);
    ctx.goTo(BuilderStep.Filter);
  };
  return (
    <SamplePanel
      preview={live}
      point={point}
      stageLabel={stage.label}
      tip={stage.showTip ? SampleStage.keepOnlyTip : undefined}
      source={live.source}
      connectionName={live.connectionName}
      valueActions={BuilderValueActions.kinds}
      onValueAction={(a) => ctx.keepOnly(a.path, a.value)}
      whySeq={ctx.whyIndex === null ? null : (live.state.records[ctx.whyIndex]?.seq ?? null)}
      onWhy={canWhy ? why : undefined}
      reasonFor={reasonFor}
    />
  );
}
