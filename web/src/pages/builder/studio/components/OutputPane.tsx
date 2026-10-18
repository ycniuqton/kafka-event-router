import { useMemo } from 'react';
import { Button, ButtonSize, ButtonVariant, EmptyState, JsonTree } from '../../../../components/ui';
import { useStudio } from '../hooks/StudioContext';
import { OutputView } from '../model/OutputView';
import { StudioText } from '../model/StudioText';
import { PaneHeader } from './PaneHeader';
import styles from './OutputPane.module.css';

/** Right column: output after all steps or at the selected step, with diff, key, headers and size. */
export function OutputPane({ className }: { className?: string }) {
  const { model, ui, commands } = useStudio();
  const { current, result } = model;
  const view = useMemo(() => (current && result ? new OutputView(current, result, ui.diff) : null), [current, result, ui.diff]);
  return (
    <section className={[styles.pane, className].filter(Boolean).join(' ')}>
      <PaneHeader title="Output" grow>
        <Button size={ButtonSize.Sm} className={styles.toggle} aria-pressed={ui.atStep} onClick={() => commands.setUi({ atStep: !ui.atStep })}>
          {ui.atStep ? 'At selected step' : 'After all steps'}
        </Button>
        <Button size={ButtonSize.Sm} className={styles.toggle} aria-pressed={ui.diff} onClick={() => commands.setUi({ diff: !ui.diff })}>
          {ui.diff ? 'Diff · on' : 'Diff · off'}
        </Button>
      </PaneHeader>
      <PreviewStatus />
      <div className={styles.body} aria-busy={model.preview.pending}>
        {!current && <EmptyState compact title="No output" description="There is no sample event to transform." />}
        {current && !view && !model.preview.failure && <div className={styles.dropped}>{StudioText.previewPending}</div>}
        {view && <OutputBody view={view} />}
      </div>
      <div className={styles.footer}>
        <div>
          key: <span className={styles.footerValue}>{view?.key}</span>
        </div>
        <div className={styles.ellipsis}>
          headers: <span className={styles.footerValue}>{view?.headers}</span>
        </div>
        <div>
          size: <span className={styles.footerValue}>{view?.size}</span>
        </div>
      </div>
    </section>
  );
}

/** Preview service state: running indicator, or the failure with Retry (output keeps the last good run). */
function PreviewStatus() {
  const { model } = useStudio();
  const { pending, failure, runNow } = model.preview;
  if (failure) {
    return (
      <div className={styles.previewFailure} role="alert">
        <span>
          {StudioText.previewFailed}: {failure.message}
        </span>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={runNow}>
          {StudioText.previewRetry}
        </Button>
      </div>
    );
  }
  return <div className={styles.previewState} data-pending={pending} title={StudioText.previewEngine} />;
}

function OutputBody({ view }: { view: OutputView }) {
  const { model } = useStudio();
  const error = view.errorMessage();
  return (
    <>
      {error && (
        <div className={styles.error}>
          <span className={styles.errorTitle}>⚠ {error}</span>
          <span className={styles.errorStep}>{view.errorStep(model.steps)} · would go to the dead-letter topic</span>
        </div>
      )}
      {view.dropped && <div className={styles.dropped}>⊘ Event dropped by a step</div>}
      {view.value !== null && <JsonTree value={view.value} className={styles.tree} decorate={(line) => view.decorate(line)} />}
      {view.removed.length > 0 && (
        <div className={styles.removed}>
          {view.removed.map((r) => (
            <div key={r.path} className={styles.removedLine}>
              <span>−</span>
              <span>{r.path}:</span>
              <span>{r.value}</span>
            </div>
          ))}
        </div>
      )}
    </>
  );
}
