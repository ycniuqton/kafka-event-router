import { useRef, type MouseEvent } from 'react';
import { JsonTree, type LineDecoration } from '../ui';
import { EventDocuments, MaskedValues, ProjectedState, SampleRecords, type JsonLine } from '../../domain';
import { MaskedMark } from './MaskedMark';
import { MaskedMarkText } from './MaskedMarkText';
import { EventStatusBadge } from './EventStatusBadge';
import type { PanelRow } from './SamplePanelModel';
import styles from './PrettyView.module.css';

export type PrettyItemProps = {
  row: PanelRow;
  open: boolean;
  highlighted: boolean;
  onToggle: () => void;
  onWhy?: () => void;
  /** Leaf value clicked (left or right button) with the pointer position; omit to make values inert. */
  onValue?: (line: JsonLine, x: number, y: number) => void;
  valueTitle?: (line: JsonLine) => LineDecoration | undefined;
  /** Values were masked by the server: masked leaves get a lock. */
  masked?: boolean;
};

/** Adds the lock of masked leaves to the caller's line decoration. */
class MaskedDecoration {
  static of(masked: boolean | undefined, base?: (line: JsonLine) => LineDecoration | undefined) {
    if (!masked) return base;
    return (line: JsonLine): LineDecoration | undefined => {
      const own = base?.(line);
      if (line.isBranch || !MaskedValues.isMasked(line.value)) return own;
      return { ...own, title: MaskedMarkText.Title, trailing: <MaskedMark /> };
    };
  }
}

/** One event in the Pretty view: status, title, coordinates, reason, "Why?" and the expandable tree. */
export function PrettyItem({ row, open, highlighted, onToggle, onWhy, onValue, valueTitle, masked }: PrettyItemProps) {
  const { record, projected } = row;
  const pointer = useRef({ x: 0, y: 0 });
  const track = (e: MouseEvent) => {
    pointer.current = { x: e.clientX, y: e.clientY };
  };
  const context = (e: MouseEvent<HTMLElement>) => {
    e.preventDefault();
    track(e);
    (e.target as HTMLElement).click();
  };
  return (
    <div className={styles.item} data-highlighted={highlighted || undefined} data-seq={record.seq}>
      <div className={styles.row} data-dim={projected.state === ProjectedState.Filtered || undefined} onClick={onToggle}>
        <EventStatusBadge state={projected.state} />
        <div className={styles.main}>
          <div className={styles.line}>
            <span className={styles.n}>#{row.position + 1}</span>
            <span className={styles.sum}>{SampleRecords.title(record)}</span>
          </div>
          <div className={styles.sub}>{SampleRecords.subtitle(record)}</div>
          {projected.reason && (
            <div className={styles.reason} data-error={projected.state === ProjectedState.Error || undefined}>
              {projected.reason}
            </div>
          )}
        </div>
        {onWhy && (
          <button type="button" className={styles.why} onClick={(e) => {
              e.stopPropagation();
              onWhy();
            }}>
            Why?
          </button>
        )}
      </div>
      {open && (
        <div className={styles.tree} onClickCapture={track} onContextMenu={onValue ? context : undefined}>
          <JsonTree
            value={EventDocuments.visible(projected.document)}
            collapsible={false}
            onLineClick={onValue ? (l: JsonLine) => !l.isBranch && onValue(l, pointer.current.x, pointer.current.y) : undefined}
            decorate={MaskedDecoration.of(masked, valueTitle)}
          />
        </div>
      )}
    </div>
  );
}
