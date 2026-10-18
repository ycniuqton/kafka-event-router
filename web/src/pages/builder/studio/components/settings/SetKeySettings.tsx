import { ValueSourceKind, type TransformStep } from '../../../../../domain';
import { StepChangeLabel } from '../../model/StudioText';
import type { StepSettingsProps } from './StepSettingsProps';
import { ValueSourceInput } from './ValueSourceInput';

/** Set key keeps a field source in `field` (as click-to-transform does) and other sources in `argument`. */
class KeySourceEdit {
  static view(step: TransformStep): TransformStep {
    return KeySourceEdit.isField(step) ? { ...step, argument: step.field } : step;
  }

  static patch(step: TransformStep, patch: Partial<TransformStep>): Partial<TransformStep> {
    if (patch.sourceKind) return { sourceKind: patch.sourceKind, argument: '', field: '' };
    return KeySourceEdit.isField(step) ? { field: patch.argument ?? '' } : patch;
  }

  private static isField(step: TransformStep): boolean {
    return (step.sourceKind ?? ValueSourceKind.Field) === ValueSourceKind.Field;
  }
}

/** Set key: from a field of the value (default), a fixed value or record metadata. */
export function SetKeySettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  return <ValueSourceInput step={KeySourceEdit.view(step)} fallback={ValueSourceKind.Field} onChange={(patch) => update(KeySourceEdit.patch(step, patch), label)} />;
}
