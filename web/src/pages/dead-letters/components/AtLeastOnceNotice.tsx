import { DeadLetterDelivery } from '../../../domain';
import styles from './AtLeastOnceNotice.module.css';

/** Warning shown for exactly-once routes: retries leave the route transaction and are delivered at-least-once (FR-21). */
export function AtLeastOnceNotice({ show }: { show: boolean }) {
  if (!show) return null;
  return (
    <div className={styles.notice} role="note" data-testid="dlq-at-least-once">
      <span className={styles.title}>{DeadLetterDelivery.title}</span>
      <span>{DeadLetterDelivery.notice}</span>
    </div>
  );
}
