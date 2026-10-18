import { Link } from 'react-router-dom';
import { services } from '../api';
import { AppPaths } from '../app/paths';
import { ToneBadge } from '../components/ui';
import { ApprovalStatus, ApprovalStatusMeta } from '../domain';
import { useServiceQuery } from '../store';
import styles from './ApprovalsMenu.module.css';

/** "Waiting for approval" badge for a route's pending deploy request, linking to its review page (History tab). */
export function PendingApprovalBanner({ routeId }: { routeId: string }) {
  const { data } = useServiceQuery(() => services.approvals.list({ routeId, status: ApprovalStatus.Pending }), [routeId]);
  const pending = data?.[0];
  if (!pending) return null;
  const meta = ApprovalStatusMeta[pending.status];
  return (
    <Link to={AppPaths.approval(pending.id)} className={styles.item}>
      <span className={styles.route}>
        v{pending.versionNo} requested by {pending.requestedBy?.name ?? 'a former member'}
      </span>
      <ToneBadge tone={meta.tone}>{meta.label}</ToneBadge>
      <span className={styles.muted}>Open the review page</span>
    </Link>
  );
}
