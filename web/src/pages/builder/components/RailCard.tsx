import type { RailCardView } from '../model/RailModel';
import styles from './RailCard.module.css';

export type RailCardProps = { card: RailCardView; active: boolean; onClick: () => void };

/** One pipeline step card: kind, dirty dot, tick or warning, one-line summary. */
export function RailCard({ card, active, onClick }: RailCardProps) {
  return (
    <button type="button" className={styles.card} data-active={active || undefined} onClick={onClick}>
      <span className={styles.head}>
        <span className={styles.kind}>{card.kind}</span>
        {card.dirty && <span className={styles.dirty} title="Unsaved" />}
        {card.warning ? <span className={styles.warn}>!</span> : <span className={styles.ok}>✓</span>}
      </span>
      <span className={styles.summary}>{card.summary}</span>
      {card.warning && <span className={styles.warnText}>{card.warning}</span>}
    </button>
  );
}
