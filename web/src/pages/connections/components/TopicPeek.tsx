import { useState } from 'react';
import { services } from '../../../api';
import { useServiceQuery } from '../../../store';
import { PeekMasking, TopicPeekConfig, TopicPeekText } from '../model/ConnectionDrawer';
import { PeekEventRow } from './PeekEventRow';
import styles from './TopicPeek.module.css';

export type TopicPeekProps = { connectionId: string; topic: string; sensitive?: boolean; canReadRaw?: boolean };

/** Last 20 events of a topic as one-line previews (masked badge when values are masked); click one to open it in the shared JSON viewer. */
export function TopicPeek({ connectionId, topic, sensitive = false, canReadRaw = true }: TopicPeekProps) {
  const { data, loading, error, reload } = useServiceQuery(() => services.samples.peek(connectionId, topic, TopicPeekConfig.count), [connectionId, topic]);
  const [open, setOpen] = useState<number | null>(null);
  if (loading) return <div className={styles.status}>Reading the latest events…</div>;
  if (error) {
    return (
      <div className={styles.status} role="alert">
        {error.message || TopicPeekText.failed}{' '}
        <button type="button" className={styles.retry} onClick={reload}>
          {TopicPeekText.retry}
        </button>
      </div>
    );
  }
  if (!data?.length) return <div className={styles.status}>No events on this topic yet.</div>;
  const masked = PeekMasking.masked(sensitive, canReadRaw);
  return (
    <div className={styles.peek}>
      {masked && (
        <div className={styles.maskedBar}>
          <span className={styles.masked} title={PeekMasking.reason(sensitive)}>
            {TopicPeekText.masked}
          </span>
          <span>{PeekMasking.reason(sensitive)}</span>
        </div>
      )}
      {data.map((event) => (
        <PeekEventRow key={event.id} event={event} open={open === event.id} onToggle={() => setOpen((o) => (o === event.id ? null : event.id))} />
      ))}
    </div>
  );
}
