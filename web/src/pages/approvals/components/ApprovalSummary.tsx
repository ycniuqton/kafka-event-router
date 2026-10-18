import { Link } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Card, EnvTag, Format, ToneBadge } from '../../../components/ui';
import { ApprovalStatusMeta, EnvTag as EnvTags, type ApprovalDetail, type EnvTag as EnvTagValue } from '../../../domain';
import { useNow } from '../../../store';
import styles from '../ApprovalReviewPage.module.css';

/** Who asked for what: route, version, requester, destinations, change note and the decision so far. */
export function ApprovalSummary({ detail }: { detail: ApprovalDetail }) {
  const a = detail.approval;
  const meta = ApprovalStatusMeta[a.status];
  const now = useNow();
  return (
    <Card padding="md" className={styles.summary}>
      <div className={styles.line}>
        <ToneBadge tone={meta.tone}>{meta.label}</ToneBadge>
        <Link to={AppPaths.routeDetail(a.routeId)} className={styles.route}>
          {a.routeName}
        </Link>
        <span className={styles.muted}>v{a.versionNo}</span>
      </div>
      <div className={styles.line}>
        Requested by <b>{a.requestedBy?.name ?? 'a former member'}</b> · {Format.relativeTime(a.requestedAt, now)}
      </div>
      <div className={styles.line}>
        Writes to
        {a.destinations.map((d) => (
          <span key={d.connectionId} className={styles.dest}>
            {d.name || d.connectionId}
            {Object.values(EnvTags).includes(d.envTag as EnvTagValue) && <EnvTag env={d.envTag as EnvTagValue} />}
          </span>
        ))}
      </div>
      {a.changeNote && <div className={styles.note}>“{a.changeNote}”</div>}
      {a.decidedBy && (
        <div className={styles.line}>
          Decided by <b>{a.decidedBy.name}</b>
          {a.decisionNote && <span className={styles.note}> — “{a.decisionNote}”</span>}
        </div>
      )}
    </Card>
  );
}
