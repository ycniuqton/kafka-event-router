import { Fragment } from 'react';
import { services } from '../../../api';
import { Card } from '../../../components/ui';
import { DiffMarkTone, RouteDiffPresenter } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { VersionCompare } from '../model/VersionCompare';
import styles from './VersionDiffView.module.css';

type VersionDiffViewProps = { routeId: string; picked: [number, number]; onClear: () => void };

/** Side-by-side readable diff of two versions by section, loaded from `GET /routes/{id}/versions/{a}/diff/{b}`. */
export function VersionDiffView({ routeId, picked, onClear }: VersionDiffViewProps) {
  const [older, newer] = VersionCompare.ordered(picked[0], picked[1]);
  const { data, error } = useServiceQuery(() => services.routes.diff(routeId, String(older), String(newer)), [routeId, older, newer]);
  const sections = data ? RouteDiffPresenter.sections(data) : [];
  return (
    <Card padding="md" className={styles.panel}>
      <div className={styles.head}>
        <span className={styles.title}>
          v{older} → v{newer}
        </span>
        <button type="button" className={styles.clear} onClick={onClear}>
          Clear
        </button>
      </div>
      <div className={styles.columns}>
        <span>v{older}</span>
        <span>v{newer}</span>
      </div>
      {sections.map((s) => (
        <Fragment key={s.section}>
          <div className={styles.section}>{s.label}</div>
          {s.rows.map((r, i) => (
            <div key={i} className={styles.row} data-tone={DiffMarkTone[r.mark]}>
              <span className={r.before ? styles.before : styles.blank}>{r.before}</span>
              <span className={r.after ? styles.after : styles.blank}>{r.after}</span>
            </div>
          ))}
        </Fragment>
      ))}
      {!data && !error && <div className={styles.note}>Loading diff…</div>}
      {error && <div className={styles.note}>Could not load the diff: {error.message}</div>}
      {data && !sections.length && <div className={styles.note}>No changes between these versions.</div>}
    </Card>
  );
}
