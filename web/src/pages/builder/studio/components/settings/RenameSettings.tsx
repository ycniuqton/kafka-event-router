import { ControlSize, TextInput } from '../../../../../components/ui';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './RecipeSettings.module.css';

/** Rename / Copy / Move: source field → target path (dots nest, e.g. `customer.id`). */
export function RenameSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  return (
    <div className={styles.row}>
      <FieldSelect className={styles.grow} value={step.field} onChange={(field) => update({ field }, label)} />
      <span className={styles.op}>→</span>
      <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder="new.path" value={step.argument} onChange={(e) => update({ argument: e.target.value }, label)} aria-label="Target field" />
    </div>
  );
}
