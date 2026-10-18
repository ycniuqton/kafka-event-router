import type { TopicInfo } from '../../../../domain';
import styles from './TopicRow.module.css';

export type TopicRowProps = { topic: TopicInfo; selected: boolean; multi: boolean; onClick: () => void };

/** Selection marks: radio for one topic, checkbox when merging several. */
class TopicMark {
  static of(selected: boolean, multi: boolean): string {
    if (multi) return selected ? '☑' : '☐';
    return selected ? '◉' : '○';
  }
}

/** A topic with its selection mark, partition count and message rate. */
export function TopicRow({ topic, selected, multi, onClick }: TopicRowProps) {
  return (
    <div className={styles.row} data-selected={selected || undefined} onClick={onClick} role="option" aria-selected={selected}>
      <span className={styles.mark}>{TopicMark.of(selected, multi)}</span>
      <span className={styles.name}>{topic.name}</span>
      <span className={styles.parts}>{topic.partitions} partitions</span>
      <span className={styles.rate}>{topic.rate}</span>
    </div>
  );
}
