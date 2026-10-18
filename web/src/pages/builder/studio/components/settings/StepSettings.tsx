import type { TransformStep } from '../../../../../domain';
import { useStudio } from '../../hooks/StudioContext';
import { StepListEditor } from '../../model/StepListEditor';
import { StepSettingsRegistry } from './StepSettingsRegistry';

/** Renders the inline settings editor registered for the step's kind. */
export function StepSettings({ step, index }: { step: TransformStep; index: number }) {
  const { model, commands } = useStudio();
  const Editor = StepSettingsRegistry.editors[step.kind];
  const update = (patch: Partial<TransformStep>, label?: string) => commands.commit(new StepListEditor(model.steps).patch(step.id, patch, label));
  return <Editor step={step} index={index} update={update} />;
}
