import { ControlSize, TextInput } from '../../../../../components/ui';
import { TransformStepKind, ValueSourceKind } from '../../../../../domain';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import { ValueSourceInput } from './ValueSourceInput';
import styles from './RecipeSettings.module.css';

/** Set value (any path, from a value source) and Default if missing (an existing field, fixed value only). */
export function SetValueSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  const isNew = step.kind === TransformStepKind.Set;
  return (
    <>
      <div className={styles.row}>
        {isNew ? (
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder="field" value={step.field} onChange={(e) => update({ field: e.target.value }, label)} aria-label="Field" />
        ) : (
          <FieldSelect className={styles.grow} value={step.field} onChange={(field) => update({ field }, label)} />
        )}
        <span className={styles.op}>{isNew ? '=' : '??'}</span>
      </div>
      {isNew ? (
        <ValueSourceInput step={step} fallback={ValueSourceKind.Constant} onChange={(patch) => update(patch, label)} />
      ) : (
        <TextInput controlSize={ControlSize.Sm} mono sunken placeholder='default, 0 or "text"' value={step.argument} onChange={(e) => update({ argument: e.target.value }, label)} aria-label="Default value" />
      )}
    </>
  );
}
