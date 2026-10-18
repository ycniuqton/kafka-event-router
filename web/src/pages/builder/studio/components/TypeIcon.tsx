import { JsonKindInfo, type JsonKind } from '../model/JsonKind';
import styles from './TypeIcon.module.css';

/** Small type badge of a JSON field (string, number, boolean, null, object, array). */
export function TypeIcon({ kind }: { kind: JsonKind }) {
  return (
    <span className={styles.icon} data-kind={kind} title={kind}>
      {JsonKindInfo.icons[kind]}
    </span>
  );
}
