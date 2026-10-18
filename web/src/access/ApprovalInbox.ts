import { useEffect, useState } from 'react';
import { services } from '../api';
import { ApprovalStatus, type Approval } from '../domain';

/** The caller's approval bell: requests waiting for them and their own recent requests. */
export type ApprovalInboxState = { awaiting: Approval[]; mine: Approval[] };

/** Refresh period of the bell (until the WS events bus pushes approval events). */
export const ApprovalInboxPollMs = 30_000;

/** How many of the caller's own requests the drawer lists. */
export const ApprovalInboxMineLimit = 10;

/** Loads the inbox: pending requests the caller did not make (when they may approve) and their own requests. */
export class ApprovalInbox {
  static async load(canApprove: boolean): Promise<ApprovalInboxState> {
    const mine = await services.approvals.list({ mine: true });
    if (!canApprove) return { awaiting: [], mine: mine.slice(0, ApprovalInboxMineLimit) };
    const own = new Set(mine.map((a) => a.id));
    const pending = await services.approvals.list({ status: ApprovalStatus.Pending });
    return { awaiting: pending.filter((a) => !own.has(a.id)), mine: mine.slice(0, ApprovalInboxMineLimit) };
  }
}

/** Polls the inbox while mounted; failures keep the last state. */
export function useApprovalInbox(canApprove: boolean): ApprovalInboxState {
  const [state, setState] = useState<ApprovalInboxState>({ awaiting: [], mine: [] });
  useEffect(() => {
    let alive = true;
    const tick = () => ApprovalInbox.load(canApprove).then((s) => alive && setState(s), () => undefined);
    tick();
    const timer = setInterval(tick, ApprovalInboxPollMs);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [canApprove]);
  return state;
}
