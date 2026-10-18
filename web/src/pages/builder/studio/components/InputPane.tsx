import { Button, ButtonSize, EmptyState, SegmentedControl, SegmentSize } from '../../../../components/ui';
import { useStudio } from '../hooks/StudioContext';
import { InputView } from '../model/StudioEnums';
import { StudioText } from '../model/StudioText';
import { InputTable } from './InputTable';
import { InputTree } from './InputTree';
import { PaneHeader } from './PaneHeader';
import styles from './InputPane.module.css';

/** Left column: the current sample, stepping through samples, tree/table toggle and click-to-transform. */
export function InputPane({ className }: { className?: string }) {
  const { model, ui, commands } = useStudio();
  const { cursor, current } = model;
  return (
    <section className={[styles.pane, className].filter(Boolean).join(' ')}>
      <PaneHeader title="Input">
        <span className={styles.eventId}>{cursor.eventId}</span>
        <Button size={ButtonSize.Sm} square aria-label="Previous event" onClick={() => commands.selectSample(cursor.previousId())}>
          ‹
        </Button>
        <span className={styles.position}>{cursor.label}</span>
        <Button size={ButtonSize.Sm} square aria-label="Next event" onClick={() => commands.selectSample(cursor.nextId())}>
          ›
        </Button>
      </PaneHeader>
      <div className={styles.sub}>
        <span className={styles.hint}>{model.samples.length} samples · click a field to mask, rename, remove or use as key</span>
        <SegmentedControl
          size={SegmentSize.Xs}
          options={[
            { value: InputView.Tree, label: 'Tree' },
            { value: InputView.Table, label: 'Table' },
          ]}
          value={ui.inputView}
          onChange={(inputView) => commands.setUi({ inputView })}
          ariaLabel="Input view"
        />
      </div>
      <div className={styles.body}>
        {!current && <EmptyState compact title="No sample events" description="Load samples from the source topic to preview transforms." />}
        {current && (ui.inputView === InputView.Tree ? <InputTree /> : <InputTable />)}
      </div>
      <div className={styles.footer}>
        key: <span className={styles.footerValue}>{current?.key ?? StudioText.noValue}</span>
      </div>
    </section>
  );
}
