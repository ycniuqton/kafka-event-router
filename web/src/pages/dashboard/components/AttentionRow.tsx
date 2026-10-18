import { Button } from '../../../components/ui';
import type { AttentionItem } from '../../../domain';
import styles from './AttentionRow.module.css';

export type AttentionRowProps = { item: AttentionItem; actionLabel: string; onAct: () => void };

/** Tone dot, title + kind, plain-language detail, and a one-click action (Open / Pause / Test…). */
export function AttentionRow({ item, actionLabel, onAct }: AttentionRowProps) {
  return (
    <div className={styles.row} data-tone={item.tone}>
      <span className={styles.dot} />
      <div className={styles.text}>
        <div className={styles.head}>
          <span className={styles.title}>{item.title}</span>
          <span className={styles.kind}>{item.kind}</span>
        </div>
        <div className={styles.detail}>{item.detail}</div>
      </div>
      <Button size="sm" className={styles.action} onClick={onAct}>
        {actionLabel}
      </Button>
    </div>
  );
}
