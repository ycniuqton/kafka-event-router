import type { TransformStep } from '../../../../domain';
import { useStudio } from '../hooks/StudioContext';
import { ExportSelection } from './ExportSelection';
import styles from './ExportToJs.module.css';

/** Checkbox on a recipe card while steps are being picked for "Export to JS". */
export function ExportCheck({ step }: { step: TransformStep }) {
  const { ui, commands } = useStudio();
  if (ui.exportSelection === null || !ExportSelection.selectable(step)) return null;
  const selection = new ExportSelection(ui.exportSelection);
  return (
    <input
      type="checkbox"
      className={styles.check}
      aria-label={`Export step ${step.id}`}
      checked={selection.has(step.id)}
      onClick={(e) => e.stopPropagation()}
      onChange={() => commands.setUi({ exportSelection: selection.toggle(step.id) })}
    />
  );
}
