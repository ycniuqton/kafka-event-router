import { Button, ButtonSize, ButtonVariant, ToneBadge } from '../../../../components/ui';
import { LookupTraceBadge, LookupTraceLine, StepKindLabel, type TransformStep } from '../../../../domain';
import type { DragHandlers } from '../hooks/useDragReorder';
import { useStudio } from '../hooks/StudioContext';
import { StepCardStateOf } from '../model/StudioEnums';
import { StepListEditor } from '../model/StepListEditor';
import { StepPresenters } from '../model/StepPresenters';
import { StudioText } from '../model/StudioText';
import { StepCardMenu } from './StepCardMenu';
import { ExportCheck } from '../export/ExportCheck';
import { LibraryStepCard } from '../library/LibraryStepCard';
import { StepSettings } from './settings/StepSettings';
import styles from './StepCard.module.css';

type StepCardProps = { step: TransformStep; index: number; drag: DragHandlers; dropTarget: boolean };

/** One step: number, title, summary, before → after example, reorder, on/off, ⋯ menu and inline settings. */
export function StepCard({ step, index, drag, dropTarget }: StepCardProps) {
  const { model, ui, commands } = useStudio();
  const presenter = StepPresenters.of(step.kind);
  const run = model.preview.run;
  const example = run && step.enabled ? presenter.example(step, run.snapshots(model.current?.id, index)) : null;
  const issue = run?.issues.get(index);
  const lookup = LookupTraceLine.first(run?.run(model.current?.id)?.steps[index]?.logs);
  const selected = ui.selectedStepId === step.id;
  const failing = (model.result?.error !== undefined && model.result.stepIndex === index) || !!issue;
  const state = StepCardStateOf.resolve(failing, selected);
  return (
    <div className={[styles.card, dropTarget && styles.dropTarget].filter(Boolean).join(' ')} data-state={state} onDragOver={drag.onDragOver} onDrop={drag.onDrop}>
      <div className={[styles.head, !step.enabled && styles.disabled].filter(Boolean).join(' ')} draggable={drag.draggable && !model.readOnly} onDragStart={drag.onDragStart} onDragEnd={drag.onDragEnd} onClick={() => commands.selectStep(selected ? null : step.id)}>
        <ExportCheck step={step} />
        <span className={styles.number} title="Drag to reorder">{index + 1}</span>
        <div className={styles.text}>
          <div className={styles.title}>
            {StepKindLabel[step.kind]}
            {model.baseline.isDirty(step) && <span className={styles.dirty} title="Unsaved change" aria-label="Unsaved change" />}
            {lookup && <span className={styles.badge}><ToneBadge tone={LookupTraceBadge[lookup.mode].tone} dot={false}>{LookupTraceBadge[lookup.mode].label}</ToneBadge></span>}
          </div>
          <div className={styles.summary}>{presenter.summary(step)}</div>
        </div>
        {!model.readOnly && <StepCardControls step={step} index={index} />}
      </div>
      <LibraryStepCard step={step} />
      {issue && (
        <div className={styles.issue} role="alert">
          {StudioText.stepIssue(issue)}
        </div>
      )}
      {example && (
        <div className={styles.example}>
          {example.before} <span className={styles.arrow}>→</span> <span className={styles.after}>{example.after}</span>
        </div>
      )}
      {selected && (
        <fieldset className={styles.settings} disabled={model.readOnly}>
          <StepSettings step={step} index={index} />
        </fieldset>
      )}
    </div>
  );
}

/** Reorder, on/off and ⋯ menu of a step card (hidden for read-only viewers). */
function StepCardControls({ step, index }: { step: TransformStep; index: number }) {
  const { model, commands } = useStudio();
  const editor = new StepListEditor(model.steps);
  return (
    <div className={styles.controls} onClick={(e) => e.stopPropagation()}>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title="Move up" disabled={index === 0} onClick={() => commands.commit(editor.move(index, index - 1))}>↑</Button>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title="Move down" disabled={index === model.steps.length - 1} onClick={() => commands.commit(editor.move(index, index + 1))}>↓</Button>
      <button type="button" className={styles.toggle} data-on={step.enabled} aria-pressed={step.enabled} onClick={() => commands.commit(editor.toggle(step))}>
        {step.enabled ? 'On' : 'Off'}
      </button>
      <StepCardMenu onDuplicate={() => commands.commit(editor.duplicate(step))} onDelete={() => commands.commit(editor.remove(step))} />
    </div>
  );
}
