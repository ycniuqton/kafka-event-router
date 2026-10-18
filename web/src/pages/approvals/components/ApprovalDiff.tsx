import { Fragment } from 'react';
import { services } from '../../../api';
import { Card, JsonTree } from '../../../components/ui';
import { DiffMarkTone, RouteDiffPresenter, type ApprovalDetail, type JsonValue } from '../../../domain';
import { useServiceQuery } from '../../../store';
import styles from '../ApprovalReviewPage.module.css';

/** What changes if approved: readable diff vs the live version, or the whole config on a first deploy. */
export function ApprovalDiff({ detail }: { detail: ApprovalDetail }) {
  const { approval, active, requested } = detail;
  if (!active) {
    return (
      <Card padding="md" className={styles.diff}>
        <div className={styles.cardTitle}>First deploy — v{requested.no} (nothing is live yet)</div>
        <JsonTree value={requested.config as JsonValue} />
      </Card>
    );
  }
  return <VersionChanges routeId={approval.routeId} from={active.no} to={requested.no} />;
}

/** Sectioned before/after rows from routes' version diff. */
function VersionChanges({ routeId, from, to }: { routeId: string; from: number; to: number }) {
  const { data, error } = useServiceQuery(() => services.routes.diff(routeId, String(from), String(to)), [routeId, from, to]);
  const sections = data ? RouteDiffPresenter.sections(data) : [];
  return (
    <Card padding="md" className={styles.diff}>
      <div className={styles.cardTitle}>
        Live v{from} → requested v{to}
      </div>
      {sections.map((s) => (
        <Fragment key={s.section}>
          <div className={styles.section}>{s.label}</div>
          {s.rows.map((r, i) => (
            <div key={i} className={styles.diffRow} data-tone={DiffMarkTone[r.mark]}>
              <span className={r.before ? styles.before : styles.blank}>{r.before}</span>
              <span className={r.after ? styles.after : styles.blank}>{r.after}</span>
            </div>
          ))}
        </Fragment>
      ))}
      {!data && !error && <div className={styles.muted}>Loading diff…</div>}
      {error && <div className={styles.muted}>Could not load the diff: {error.message}</div>}
      {data && !sections.length && <div className={styles.muted}>No changes between these versions.</div>}
    </Card>
  );
}
