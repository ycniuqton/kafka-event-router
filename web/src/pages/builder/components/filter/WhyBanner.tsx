import styles from './WhyBanner.module.css';

export type WhyBannerProps = {
  index: number;
  passed: boolean;
  onClear: () => void;
};

/** Explains which sample's results the condition rows are showing. */
export function WhyBanner({ index, passed, onClear }: WhyBannerProps) {
  return (
    <div className={styles.banner}>
      <span className={styles.text}>
        Showing why event #{index + 1} {passed ? 'passed' : 'was filtered out'}
      </span>
      <button type="button" className={styles.clear} onClick={onClear}>
        Clear
      </button>
    </div>
  );
}
