import type { JsonLine } from '../../../../domain';
import { JsonLineKind } from '../model/JsonKind';
import styles from './JsonValueText.module.css';

/** A JSON line's value coloured by type, or the `{n}` / `[n]` summary of a branch. */
export function JsonValueText({ line }: { line: JsonLine }) {
  if (line.isBranch) return <span className={styles.summary}>{line.summary}</span>;
  return (
    <span className={styles.value} data-kind={JsonLineKind.of(line)}>
      {JSON.stringify(line.value)}
    </span>
  );
}
