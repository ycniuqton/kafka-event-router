import { Format, JsonTree } from '../../../components/ui';
import { TailEncoding, TailOutcomeMeta, Tone, type TailEvent } from '../../../domain';
import { TailPayloadText, type TailRow as TailRowModel } from './TailBuffer';
import styles from './TailTab.module.css';

type TailRowProps = { row: TailRowModel; open: boolean; onToggle: () => void };

/** One tailed event: compact input → output line; click opens the split input/output view. */
export function TailRow({ row, open, onToggle }: TailRowProps) {
  const { record } = row;
  const meta = TailOutcomeMeta[record.outcome];
  const output = record.outputs[0];
  const outputText = output ? TailPayloadText.summary(output.value) : (record.error ?? meta.label);
  return (
    <>
      <button type="button" className={`${styles.grid} ${styles.row}`} onClick={onToggle} aria-expanded={open} data-testid="tail-row">
        <span className={styles.muted}>{Format.clockTime(record.at)}</span>
        <span className={styles.muted}>{record.offset}</span>
        <span data-tone={meta.tone} className={styles.toned} title={meta.label}>
          {meta.icon}
        </span>
        <span className={styles.input}>{TailPayloadText.summary(record.input.value)}</span>
        <span data-tone={output ? Tone.Text : meta.tone} className={styles.toned}>
          {outputText}
        </span>
      </button>
      {open && <TailSplitView row={row} />}
    </>
  );
}

/** Input on the left, outputs (or the failure) on the right. */
function TailSplitView({ row }: { row: TailRowModel }) {
  const { record } = row;
  const meta = TailOutcomeMeta[record.outcome];
  return (
    <div className={styles.detail}>
      <div className={styles.pane}>
        <span className={styles.paneTitle}>
          Input · {record.input.topic} · p{record.partition} · offset {record.offset}
        </span>
        <TailPayload event={record.input} />
      </div>
      <div className={styles.pane}>
        <span className={styles.paneTitle}>Output · {meta.label}</span>
        {record.outputs.map((o, i) => (
          <TailPayload key={`${o.topic}-${i}`} event={o} />
        ))}
        {record.error && (
          <span className={styles.error} data-tone={meta.tone}>
            {[record.step, record.errorType, record.error].filter(Boolean).join(' · ')}
          </span>
        )}
      </div>
    </div>
  );
}

function TailPayload({ event }: { event: TailEvent }) {
  if (event.encoding === TailEncoding.Json) return <JsonTree value={event.value} />;
  return <span className={styles.input}>{String(event.value)}</span>;
}
