import { ToneBadge } from '../../../components/ui';
import { DeadLetterErrorClassMeta, type DeadLetterErrorClass } from '../../../domain';
import styles from './ErrorClassBadge.module.css';

export type ErrorClassBadgeProps = { errorClass: DeadLetterErrorClass };

/** "Bad data" / "Router bug" / "Transient" pill; the hint says what to do about it. */
export function ErrorClassBadge({ errorClass }: ErrorClassBadgeProps) {
  const meta = DeadLetterErrorClassMeta[errorClass];
  return (
    <span className={styles.badge} title={meta.hint} data-class={errorClass}>
      <ToneBadge tone={meta.tone} dot={false}>
        {meta.label}
      </ToneBadge>
    </span>
  );
}
