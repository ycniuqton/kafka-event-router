import { SegmentedControl, SegmentSize } from '../../../../../components/ui';
import { MaskMode, MaskModeLabel } from '../../../../../domain';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './RecipeSettings.module.css';

/** Mask: field plus style (Email · Keep last 4 · Full). */
export function MaskSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  const modes = Object.values(MaskMode).map((m) => ({ value: m, label: MaskModeLabel[m] }));
  return (
    <>
      <FieldSelect value={step.field} onChange={(field) => update({ field }, label)} />
      <div className={styles.start}>
        <SegmentedControl size={SegmentSize.Xs} options={modes} value={step.maskMode} onChange={(maskMode) => update({ maskMode }, label)} ariaLabel="Mask style" />
      </div>
    </>
  );
}
