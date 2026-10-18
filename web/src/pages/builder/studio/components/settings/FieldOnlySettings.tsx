import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';

/** Steps configured by a single field (Hash). */
export function FieldOnlySettings({ step, update }: StepSettingsProps) {
  return <FieldSelect value={step.field} onChange={(field) => update({ field }, StepChangeLabel.edited(step.kind))} />;
}
