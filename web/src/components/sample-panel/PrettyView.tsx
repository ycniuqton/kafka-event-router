import { useState } from 'react';
import type { LineDecoration } from '../ui';
import type { JsonLine } from '../../domain';
import { PrettyItem } from './PrettyItem';
import type { PanelRow } from './SamplePanelModel';

export type PrettyViewProps = {
  rows: PanelRow[];
  whySeq?: number | null;
  onWhy?: (seq: number) => void;
  onValue?: (row: PanelRow, line: JsonLine, x: number, y: number) => void;
  valueTitle?: (line: JsonLine) => LineDecoration | undefined;
  masked?: boolean;
};

/** Pretty view: one collapsible JSON tree per event (first one open), statuses and "Why?". */
export function PrettyView({ rows, whySeq, onWhy, onValue, valueTitle, masked }: PrettyViewProps) {
  const [toggled, setToggled] = useState<ReadonlySet<number>>(() => new Set());
  const firstSeq = rows[0]?.record.seq;
  const isOpen = (seq: number) => (seq === firstSeq) !== toggled.has(seq);
  const toggle = (seq: number) =>
    setToggled((prev) => {
      const next = new Set(prev);
      if (!next.delete(seq)) next.add(seq);
      return next;
    });
  return (
    <>
      {rows.map((row) => (
        <PrettyItem
          key={row.record.seq}
          row={row}
          open={isOpen(row.record.seq)}
          highlighted={whySeq === row.record.seq}
          onToggle={() => toggle(row.record.seq)}
          onWhy={onWhy && (() => onWhy(row.record.seq))}
          onValue={onValue && ((line, x, y) => onValue(row, line, x, y))}
          valueTitle={valueTitle}
          masked={masked}
        />
      ))}
    </>
  );
}
