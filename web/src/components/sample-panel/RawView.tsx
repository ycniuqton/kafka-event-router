import { ProjectedState, RawEncoding } from '../../domain';
import { EventStatusBadge } from './EventStatusBadge';
import { MaskedMark } from './MaskedMark';
import type { PanelRow } from './SamplePanelModel';
import styles from './RawView.module.css';

/** Raw text of one event: exact bytes as read, or the JSON of the event after the selected step. */
class RawText {
  static value(row: PanelRow): string {
    return row.projected.changed ? JSON.stringify(row.projected.document.value) : row.record.raw.value;
  }

  static headers(row: PanelRow): string {
    return row.projected.document.headers.map((h) => `${h.key}=${h.value}`).join(' · ');
  }
}

export type RawViewProps = { rows: PanelRow[]; masked?: boolean };

/** Raw view: key, headers and the value text (encoding noted when not UTF-8; masked sessions show masked JSON). */
export function RawView({ rows, masked }: RawViewProps) {
  return (
    <>
      {rows.map((row) => (
        <div key={row.record.seq} className={styles.item} data-dim={row.projected.state === ProjectedState.Filtered || undefined}>
          <div className={styles.meta}>
            <EventStatusBadge state={row.projected.state} />
            <span>#{row.position + 1}</span>
            <span>key: {row.record.raw.key ?? '∅'}</span>
            {row.record.raw.encoding !== RawEncoding.Utf8 && <span>({row.record.raw.encoding})</span>}
            {masked && <MaskedMark label />}
          </div>
          {row.projected.document.headers.length > 0 && <div className={styles.headers}>{RawText.headers(row)}</div>}
          <div className={styles.value}>{RawText.value(row)}</div>
        </div>
      ))}
    </>
  );
}
