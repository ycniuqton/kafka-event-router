import { Format, Mono, TableCell, TableRow, ToneBadge } from '../../../components/ui';
import { DeadLetterClasses, DeadLetterResolutionMeta, Tone, type DeadLetter } from '../../../domain';
import { ErrorClassBadge } from './ErrorClassBadge';
import styles from './DeadLetterRow.module.css';

export type DeadLetterRowProps = {
  deadLetter: DeadLetter;
  routeName: string;
  selected: boolean;
  checked: boolean;
  onSelect: () => void;
  onToggle: () => void;
};

/** One failed event: time, error type, short message, route, failing step and source offset. */
export function DeadLetterRow({ deadLetter: d, routeName, selected, checked, onSelect, onToggle }: DeadLetterRowProps) {
  return (
    <TableRow selected={selected} onClick={onSelect} className={styles.row}>
      <input
        type="checkbox"
        className={styles.check}
        checked={checked}
        aria-label={`Select event at offset ${d.offset}`}
        onClick={(e) => e.stopPropagation()}
        onChange={onToggle}
      />
      <Mono tone={Tone.Muted}>{Format.clockTime(d.failedAt)}</Mono>
      <span className={styles.kind}>{d.kind}</span>
      <div className={styles.body}>
        <TableCell>
          {d.resolution && (
            <span className={styles.resolution} title={`by ${d.resolution.by} · ${Format.monthDayTime(d.resolution.at)}`}>
              <ToneBadge tone={DeadLetterResolutionMeta[d.resolution.state].tone}>{DeadLetterResolutionMeta[d.resolution.state].label}</ToneBadge>
            </span>
          )}
          <ErrorClassBadge errorClass={DeadLetterClasses.of(d)} />
          <Mono>{d.message}</Mono>
        </TableCell>
        <div className={styles.meta}>
          {routeName} · {d.step} · p{d.partition}@{d.offset}
          {d.attempts ? ` · ${d.attempts} auto-retries` : ''}
        </div>
      </div>
    </TableRow>
  );
}
