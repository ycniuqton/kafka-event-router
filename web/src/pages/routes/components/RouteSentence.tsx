import { Mono } from '../../../components/ui';
import { Tone } from '../../../domain';
import type { RouteRowModel } from '../model/RouteListModel';
import styles from './RouteSentence.module.css';

/** The route read as a sentence: topic → filter → transform → destination. */
export function RouteSentence({ row }: { row: RouteRowModel }) {
  return (
    <div className={styles.sentence}>
      <span className={styles.topic} title={row.sourceText}>{row.topicText}</span>
      <span className={styles.arrow}>→</span>
      <span className={styles.filter} title="Filter">{row.route.filterSummary}</span>
      <span className={styles.arrow}>→</span>
      <span className={styles.transform} data-tone={row.transformTone} title="Transform">{row.transformLabel}</span>
      <span className={styles.arrow}>→</span>
      <Mono tone={Tone.Text} title={row.destinationText}>{row.route.destination.topic}</Mono>
    </div>
  );
}
