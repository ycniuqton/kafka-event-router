import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import type { JsonLine } from '../../../../domain';
import { useStudio } from '../hooks/StudioContext';
import { FieldActionCatalog, type FieldAction } from '../model/FieldActions';
import { StepListEditor } from '../model/StepListEditor';
import styles from './FieldActionBar.module.css';

/** Click-to-transform buttons under a selected Input field: Mask · Rename · Remove · Use as key. */
export function FieldActionBar({ line }: { line: JsonLine }) {
  const { model, commands } = useStudio();
  const run = (action: FieldAction) => {
    const step = action.build(line);
    commands.commit(new StepListEditor(model.steps).insertAfter(model.selectedStepIndex, [step], action.changeLabel(line.path)));
    commands.setUi({ selectedStepId: step.id, selectedLine: null });
  };
  if (model.readOnly) return null;
  return (
    <div className={styles.bar} onClick={(e) => e.stopPropagation()}>
      {FieldActionCatalog.all.map((action) => (
        <Button key={action.id} size={ButtonSize.Sm} variant={ButtonVariant.Secondary} onClick={() => run(action)}>
          {action.label}
        </Button>
      ))}
    </div>
  );
}
