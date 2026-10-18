import { EnvTag } from './connection';
import { Tone } from './tone';

/** Status of an approval request (Postgres `deployment_status`). */
export const ApprovalStatus = {
  Pending: 'pending_approval',
  RollingOut: 'rolling_out',
  Active: 'active',
  Failed: 'failed',
  Superseded: 'superseded',
  Rejected: 'rejected',
  Cancelled: 'cancelled',
} as const;
export type ApprovalStatus = (typeof ApprovalStatus)[keyof typeof ApprovalStatus];

/** Why a pending request was cancelled. */
export const CancelReason = { Withdrawn: 'withdrawn', Superseded: 'superseded' } as const;
export type CancelReason = (typeof CancelReason)[keyof typeof CancelReason];

/** Label and tone of each approval status (badges on the review page, drawer and History tab). */
export const ApprovalStatusMeta: Readonly<Record<ApprovalStatus, { label: string; tone: Tone }>> = {
  pending_approval: { label: 'Waiting for approval', tone: Tone.Warn },
  rolling_out: { label: 'Approved · rolling out', tone: Tone.Info },
  active: { label: 'Approved · live', tone: Tone.Ok },
  failed: { label: 'Approved · rollout failed', tone: Tone.Err },
  superseded: { label: 'Superseded', tone: Tone.Off },
  rejected: { label: 'Rejected', tone: Tone.Err },
  cancelled: { label: 'Withdrawn', tone: Tone.Off },
};

/** A person shown on an approval. */
export type ApprovalPerson = { id: string; name: string };

/** One branch destination of the requested version. */
export type ApprovalDestination = { connectionId: string; name: string; envTag: EnvTag | string };

/** One approval request (`GET /approvals`). */
export type Approval = {
  id: string;
  routeId: string;
  routeName: string;
  teamId: string;
  versionId: string;
  versionNo: number;
  status: ApprovalStatus;
  requestedBy: ApprovalPerson | null;
  requestedAt: string;
  changeNote: string;
  decidedBy: ApprovalPerson | null;
  decidedAt: string | null;
  decisionNote: string;
  cancelReason?: CancelReason;
  destinations: ApprovalDestination[];
  rowVersion: number;
};

/** A review comment. */
export type ApprovalComment = { id: string; approvalId: string; author: ApprovalPerson | null; body: string; createdAt: string };

/** A version's number and configuration. */
export type ApprovalVersion = { no: number; config: unknown };

/** The review page (`GET /approvals/{id}`). */
export type ApprovalDetail = {
  approval: Approval;
  requested: ApprovalVersion;
  active: ApprovalVersion | null;
  comments: ApprovalComment[];
  canDecide: boolean;
  canWithdraw: boolean;
  eligibleApprovers: number;
};

/** List filter of `GET /approvals`. */
export type ApprovalFilter = { status?: ApprovalStatus; teamId?: string; routeId?: string; mine?: boolean };

/** Limits of review input (same as the API). */
export const ApprovalLimits = { maxComment: 4000 } as const;

/** Business rules of approvals shown in the UI (the server re-checks every one). */
export class ApprovalRules {
  static isPending(a: Approval): boolean {
    return a.status === ApprovalStatus.Pending;
  }

  /** Requests needing the caller's decision: pending and not requested by the caller. */
  static awaiting(items: readonly Approval[], userId: string | null): Approval[] {
    return items.filter((a) => ApprovalRules.isPending(a) && a.requestedBy?.id !== userId);
  }

  /** A comment body is 1–4000 characters after trimming. */
  static validComment(body: string): boolean {
    const text = body.trim();
    return text.length > 0 && text.length <= ApprovalLimits.maxComment;
  }

  /** A rejection must explain itself. */
  static validRejectNote(note: string): boolean {
    return note.trim().length > 0;
  }

  static hasProdDestination(a: Approval): boolean {
    return a.destinations.some((d) => d.envTag === EnvTag.Prod);
  }
}
