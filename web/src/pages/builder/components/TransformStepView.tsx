import { useMemo } from 'react';
import { DirtyKey } from '../model/DraftSession';
import { PipelinePreview } from '../model/PipelinePreview';
import { TransformStudio } from '../studio/TransformStudio';
import type { StepEditorProps } from './StepEditorProps';

/** Transform step: hands the draft's steps and the filtered samples to the Transform Studio. */
export function TransformStepView({ ctx }: StepEditorProps) {
  const { samples, preview, editor } = ctx;
  const studioSamples = useMemo(() => PipelinePreview.studioSamples(samples, preview), [samples, preview]);
  return (
    <TransformStudio
      steps={ctx.draft.steps}
      onStepsChange={(steps, changeLabel) => editor.setSteps(steps, changeLabel)}
      samples={studioSamples}
      selectedSampleId={ctx.selectedSampleId}
      onSelectSample={ctx.selectSample}
      stepsSaved={!ctx.dirty.has(DirtyKey.Transform)}
      routeId={ctx.draft.routeId}
      connectionId={ctx.draft.source.connectionId}
      sampleSessionId={ctx.live.state.session?.id ?? null}
    />
  );
}
