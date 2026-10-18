import { useState } from 'react';
import { services } from '../../../../api';
import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { actions } from '../../../../store';
import { useStudio } from '../hooks/StudioContext';
import { StepListEditor } from '../model/StepListEditor';
import { StepChangeLabel } from '../model/StudioText';
import { ExportSelection } from './ExportSelection';
import { ExportText } from './ExportText';
import styles from './ExportToJs.module.css';

/** Header button that starts selecting steps for export (editors only). */
export function ExportToJsButton() {
  const { model, ui, commands } = useStudio();
  if (model.readOnly || ui.exportSelection !== null || !model.steps.some(ExportSelection.selectable)) return null;
  return (
    <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} title={ExportText.startTitle} onClick={() => commands.setUi({ exportSelection: [] })}>
      {ExportText.start}
    </Button>
  );
}

/** Selection bar: count, Convert (replaces the selection with one js step) and Cancel. */
export function ExportToJsBar() {
  const { model, ui, commands } = useStudio();
  const [busy, setBusy] = useState(false);
  if (ui.exportSelection === null) return null;
  const selection = new ExportSelection(ui.exportSelection);
  const convert = async () => {
    const picked = selection.steps(model.steps);
    setBusy(true);
    try {
      const js = await services.assist.exportJs(picked);
      commands.commit(new StepListEditor(model.steps).replace(selection.idSet, js, StepChangeLabel.exported(picked.length)));
      commands.setUi({ exportSelection: null, selectedStepId: js.id });
    } catch (e) {
      actions.toast.show((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className={styles.bar} role="toolbar" aria-label={ExportText.start}>
      <span>{ExportText.prompt}</span>
      <span className={styles.count}>{ExportText.selected(selection.size)}</span>
      <span className={styles.spacer} />
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Primary} disabled={!selection.size || busy} onClick={() => void convert()}>
        {busy ? ExportText.converting : ExportText.convert}
      </Button>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => commands.setUi({ exportSelection: null })}>
        {ExportText.cancel}
      </Button>
    </div>
  );
}
