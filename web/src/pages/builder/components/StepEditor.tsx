import type { ReactNode } from 'react';
import { BuilderStep } from '../../../domain';
import { DestinationStep } from './destination/DestinationStep';
import { FilterStep } from './filter/FilterStep';
import { ReviewStep } from './review/ReviewStep';
import { SourceStep } from './source/SourceStep';
import type { StepEditorProps } from './StepEditorProps';
import { TransformStepView } from './TransformStepView';

/** Which editor renders each builder step (the Transform Studio at step=transform). */
class StepEditorRegistry {
  private static readonly editors: Record<BuilderStep, (props: StepEditorProps) => ReactNode> = {
    [BuilderStep.Source]: (p) => <SourceStep {...p} />,
    [BuilderStep.Filter]: (p) => <FilterStep {...p} />,
    [BuilderStep.Transform]: (p) => <TransformStepView {...p} />,
    [BuilderStep.Destination]: (p) => <DestinationStep {...p} />,
    [BuilderStep.Review]: (p) => <ReviewStep {...p} />,
  };

  static render(props: StepEditorProps): ReactNode {
    return StepEditorRegistry.editors[props.ctx.step](props);
  }
}

/** Centre zone: the editor of the selected step. */
export function StepEditor(props: StepEditorProps) {
  return StepEditorRegistry.render(props);
}
