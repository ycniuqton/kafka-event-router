import { Format } from '../../../components/ui';
import type { Activity } from '../../../domain';
import styles from './ActivityRow.module.css';

/** "<who> <verb> <route> <detail>" with a relative time on the right. */
export function ActivityRow({ activity, now }: { activity: Activity; now: Date }) {
  return (
    <div className={styles.row}>
      <span className={styles.line}>
        <span className={styles.actor}>{activity.actor}</span> {activity.verb} <span className={styles.route}>{activity.routeName}</span>{' '}
        <span className={styles.detail}>{activity.detail}</span>
      </span>
      <span className={styles.when}>{Format.relativeTime(activity.at, now)}</span>
    </div>
  );
}
