import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../app/paths';
import { Button, Drawer, ToneBadge } from '../components/ui';
import { ApprovalStatusMeta, Permission, type Approval } from '../domain';
import { useApprovalInbox } from './ApprovalInbox';
import { useCan } from './useCan';
import styles from './ApprovalsMenu.module.css';

/** Top-bar "Approvals N" button: N requests wait for the caller; the drawer links each to its review page. */
export function ApprovalsMenu() {
  const canApprove = useCan(Permission.DeploymentApprove);
  const inbox = useApprovalInbox(canApprove);
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const go = (a: Approval) => {
    setOpen(false);
    navigate(AppPaths.approval(a.id));
  };
  if (!canApprove && inbox.mine.length === 0) return null;
  return (
    <>
      <Button size="lg" onClick={() => setOpen(true)} aria-label="Approvals">
        Approvals {inbox.awaiting.length > 0 && <span className={styles.count}>{inbox.awaiting.length}</span>}
      </Button>
      <Drawer open={open} onClose={() => setOpen(false)} title="Deploy approvals">
        <ApprovalList title="Waiting for you" empty="Nothing waits for your approval." items={inbox.awaiting} onOpen={go} />
        <ApprovalList title="Your requests" empty="You have no deploy requests." items={inbox.mine} onOpen={go} />
      </Drawer>
    </>
  );
}

type ApprovalListProps = { title: string; empty: string; items: Approval[]; onOpen: (a: Approval) => void };

/** One group of approval entries (route, version, requester, status). */
export function ApprovalList({ title, empty, items, onOpen }: ApprovalListProps) {
  return (
    <section className={styles.group}>
      <div className={styles.groupTitle}>{title}</div>
      {items.length === 0 && <div className={styles.empty}>{empty}</div>}
      {items.map((a) => (
        <button key={a.id} type="button" className={styles.item} onClick={() => onOpen(a)}>
          <span className={styles.route}>
            {a.routeName} <span className={styles.muted}>v{a.versionNo}</span>
          </span>
          <ToneBadge tone={ApprovalStatusMeta[a.status].tone}>{ApprovalStatusMeta[a.status].label}</ToneBadge>
          <span className={styles.muted}>by {a.requestedBy?.name ?? 'a former member'}</span>
        </button>
      ))}
    </section>
  );
}
