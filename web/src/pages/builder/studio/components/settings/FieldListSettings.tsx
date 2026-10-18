import { Button, ButtonSize, ButtonVariant } from '../../../../../components/ui';
import { StepFieldList } from '../../../../../domain';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './RecipeSettings.module.css';

/** Remove / Keep only: a list of fields, each picked from the sample, plus "+ Add field". */
export function FieldListSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  const fields = StepFieldList.of(step);
  const save = (next: string[]) => update({ fields: next, field: next[0] ?? '' }, label);
  return (
    <>
      {fields.map((path, i) => (
        <div key={i} className={styles.row}>
          <FieldSelect className={styles.grow} value={path} onChange={(value) => save(fields.map((f, j) => (j === i ? value : f)))} />
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square aria-label="Remove field from list" onClick={() => save(fields.filter((_, j) => j !== i))}>
            ×
          </Button>
        </div>
      ))}
      <div className={styles.start}>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => save([...fields, ''])}>
          + Add field
        </Button>
      </div>
    </>
  );
}
