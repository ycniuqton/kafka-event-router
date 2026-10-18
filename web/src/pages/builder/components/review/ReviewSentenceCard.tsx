import type { ReviewSentence } from '../../model/ReviewModel';
import styles from './ReviewSentenceCard.module.css';

/** "From …, keep events where …, reshape with …, and send them to …." */
export function ReviewSentenceCard({ sentence }: { sentence: ReviewSentence }) {
  return (
    <div className={styles.card}>
      {sentence.branch && <>On {sentence.branch}: </>}
      From <span className={styles.code}>{sentence.source}</span>, keep events where <span className={styles.code}>{sentence.filter}</span>, reshape with{' '}
      <span className={styles.pill}>{sentence.transform}</span>, and send them to <span className={styles.code}>{sentence.destination}</span>.
    </div>
  );
}
