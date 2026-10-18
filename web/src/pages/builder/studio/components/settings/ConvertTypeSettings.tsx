import { ControlSize, Select } from '../../../../../components/ui';
import { ConvertFailure, ConvertFailureLabel, ConvertTarget, ConvertTargetLabel } from '../../../../../domain';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './RecipeSettings.module.css';

/** Convert type: field → Text / Number / Integer / Boolean, and what to do when a value does not convert. */
export function ConvertTypeSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  return (
    <>
      <div className={styles.row}>
        <FieldSelect className={styles.grow} value={step.field} onChange={(field) => update({ field }, label)} />
        <span className={styles.op}>→</span>
        <Select controlSize={ControlSize.Sm} sunken value={step.targetType ?? ConvertTarget.Number} onChange={(e) => update({ targetType: e.target.value as ConvertTarget }, label)} aria-label="Target type">
          {Object.values(ConvertTarget).map((t) => (
            <option key={t} value={t}>
              {ConvertTargetLabel[t]}
            </option>
          ))}
        </Select>
      </div>
      <div className={styles.row}>
        <span className={styles.op}>If it does not convert</span>
        <Select controlSize={ControlSize.Sm} sunken className={styles.grow} value={step.onFailure ?? ConvertFailure.Error} onChange={(e) => update({ onFailure: e.target.value as ConvertFailure }, label)} aria-label="On conversion failure">
          {Object.values(ConvertFailure).map((f) => (
            <option key={f} value={f}>
              {ConvertFailureLabel[f]}
            </option>
          ))}
        </Select>
      </div>
    </>
  );
}
