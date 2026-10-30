import { JsonTree, Mono, ToneBadge } from '../../../components/ui';
import { DryRunOutcome, DryRunOutcomeLabel, Tone, type DiffEntry, type DryRunResult } from '../../../domain';
import styles from './DryRunResults.module.css';

/** Tone of each dry-run outcome. */
const OutcomeTone: Record<DryRunOutcome, Tone> = { [DryRunOutcome.WouldSend]: Tone.Ok, [DryRunOutcome.Filtered]: Tone.Off, [DryRunOutcome.Failed]: Tone.Err };

/** One dry-run result: outcome, the new error, or the outputs per branch (masked like the source view). */
export function DryRunResultView({ result }: { result: DryRunResult }) {
  return (
    <div className={styles.result}>
      <div className={styles.head}>
        <ToneBadge tone={OutcomeTone[result.outcome]}>{DryRunOutcomeLabel[result.outcome]}</ToneBadge>
        {result.message && <Mono size="sm">{result.message}</Mono>}
      </div>
      {result.outputs.map((o, i) => (
        <div key={i} className={styles.output}>
          <div className={styles.meta}>
            branch {o.branch} → <Mono size="sm">{o.topic || o.connectionId}</Mono>
          </div>
          <JsonTree value={{ key: o.key, value: o.value, headers: o.headers }} />
        </div>
      ))}
    </div>
  );
}

/** Paths an edit changes (values are never shown here). */
export function DiffList({ diff }: { diff: DiffEntry[] }) {
  if (!diff.length) return <div className={styles.meta}>No changes: the event is replayed as it is.</div>;
  return (
    <ul className={styles.diff} aria-label="Changes">
      {diff.map((d) => (
        <li key={`${d.op}${d.path}`}>
          <span className={styles.op} data-op={d.op}>
            {d.op}
          </span>
          <Mono size="sm">{d.path}</Mono>
        </li>
      ))}
    </ul>
  );
}
