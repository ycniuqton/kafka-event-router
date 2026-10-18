import { MaskedMarkText } from './MaskedMarkText';
import styles from './MaskedMark.module.css';

/** Lock icon (with an optional "masked" label) next to values the preview service masked. */
export function MaskedMark({ label = false }: { label?: boolean }) {
  return (
    <span className={styles.mark} title={MaskedMarkText.Title} data-testid="masked-mark">
      <svg viewBox="0 0 12 12" className={styles.icon} aria-hidden>
        <rect x="2" y="5.5" width="8" height="5.5" rx="1" />
        <path d="M4 5.5V4a2 2 0 0 1 4 0v1.5" fill="none" />
      </svg>
      {label && <span>{MaskedMarkText.Label}</span>}
    </span>
  );
}
