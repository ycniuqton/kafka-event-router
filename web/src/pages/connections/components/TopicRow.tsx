import { Button, Mono } from '../../../components/ui';
import { Tone, type TopicInfo } from '../../../domain';
import { TopicPeek } from './TopicPeek';
import styles from './TopicRow.module.css';

export type TopicRowProps = { connectionId: string; topic: TopicInfo; peeking: boolean; onPeek: () => void; sensitive?: boolean; canReadRaw?: boolean };

/** A topic card: name, partitions, replication, rate and a Peek toggle that expands the latest events. */
export function TopicRow({ connectionId, topic, peeking, onPeek, sensitive, canReadRaw }: TopicRowProps) {
  return (
    <div className={styles.card}>
      <div className={styles.head}>
        <Mono size="md" truncate className={styles.name}>
          {topic.name}
        </Mono>
        {topic.internal && <span className={styles.badge}>internal</span>}
        <span className={styles.parts}>{topic.partitions} part.</span>
        {topic.replicationFactor !== undefined && <span className={styles.parts}>RF {topic.replicationFactor}</span>}
        <Mono tone={Tone.TextSecondary}>{topic.rate}</Mono>
        <Button size="sm" onClick={onPeek} aria-expanded={peeking}>
          Peek
        </Button>
      </div>
      {peeking && <TopicPeek connectionId={connectionId} topic={topic.name} sensitive={sensitive} canReadRaw={canReadRaw} />}
    </div>
  );
}
