import { JsonTree } from '../../../components/ui';
import type { SampleEvent } from '../../../domain';
import { TopicPeekConfig } from '../model/ConnectionDrawer';
import styles from './TopicPeek.module.css';

export type PeekEventRowProps = { event: SampleEvent; open: boolean; onToggle: () => void };

/** One peeked event: offset + truncated JSON line, expanding into the shared JSON viewer. */
export function PeekEventRow({ event, open, onToggle }: PeekEventRowProps) {
  return (
    <div className={styles.row}>
      <button type="button" className={styles.line} onClick={onToggle} aria-expanded={open}>
        <span className={styles.offset}>{event.offset} </span>
        {JSON.stringify(event.value).slice(0, TopicPeekConfig.previewChars)}
      </button>
      {open && <JsonTree className={styles.tree} value={{ key: event.key, headers: event.headers, value: event.value }} />}
    </div>
  );
}
