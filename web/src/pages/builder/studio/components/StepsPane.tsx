import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { useStudio } from '../hooks/StudioContext';
import { useDragReorder } from '../hooks/useDragReorder';
import { StepListEditor } from '../model/StepListEditor';
import { StudioText } from '../model/StudioText';
import { AddStepMenu } from './AddStepMenu';
import { AssistPrompt } from '../magic/AssistPrompt';
import { ByExampleDialog } from '../magic/ByExampleDialog';
import { ByExampleText } from '../magic/MagicText';
import { ExplainChain, ExplainChainButton } from '../magic/ExplainChain';
import { LibraryLatestContext, useLibraryLatestLoader } from '../library/LibraryLatest';
import { PaneHeader } from './PaneHeader';
import { SaveToLibrary } from './SaveToLibrary';
import { StepCard } from './StepCard';
import { TestCasesPanel } from '../tests/TestCasesPanel';
import { ExportToJsBar, ExportToJsButton } from '../export/ExportToJsBar';
import styles from './StepsPane.module.css';

/** Middle column: run summary, AI prompt, step cards (drag to reorder) and the add-step menu. */
export function StepsPane({ className }: { className?: string }) {
  const { model, commands } = useStudio();
  const { report, steps } = model;
  const drag = useDragReorder((from, to) => commands.commit(new StepListEditor(steps).move(from, to)));
  const jumpTarget = report.jumpTarget(model.current?.id);
  const latest = useLibraryLatestLoader(steps);
  return (
    <section className={[styles.pane, className].filter(Boolean).join(' ')}>
      <PaneHeader title="Transform steps" grow raised>
        <span className={styles.summary} data-failing={report.errorCount > 0}>
          {model.preview.run ? report.summary : StudioText.previewPending}
        </span>
        {jumpTarget !== null && (
          <button type="button" className={styles.jump} onClick={() => commands.selectSample(jumpTarget)}>
            {StudioText.jumpTo(jumpTarget)}
          </button>
        )}
        <ExportToJsButton />
        <SaveToLibrary />
      </PaneHeader>
      <div className={styles.body}>
        {model.readOnly ? <div className={styles.readOnly}>{StudioText.readOnly}</div> : <AssistPrompt />}
        <div className={styles.magic}>
          {!model.readOnly && (
            <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => commands.setUi({ byExample: true })}>
              {ByExampleText.open}
            </Button>
          )}
          <ExplainChainButton />
        </div>
        <ExplainChain />
        <ExportToJsBar />
        {!steps.length && <div className={styles.empty}>No steps — events pass through unchanged.</div>}
        <LibraryLatestContext.Provider value={latest}>
          {steps.map((step, index) => (
            <StepCard key={step.id} step={step} index={index} drag={drag.handlersFor(index)} dropTarget={drag.overIndex === index && drag.dragIndex !== index} />
          ))}
        </LibraryLatestContext.Provider>
        {!model.readOnly && <AddStepMenu />}
        <TestCasesPanel />
      </div>
      <ByExampleDialog />
    </section>
  );
}
