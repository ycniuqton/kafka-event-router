import { ApprovalRules, type ApprovalDetail } from '../../../domain';

/** Which decision the bar is composing. */
export const DecisionKind = { Approve: 'approve', Reject: 'reject', Withdraw: 'withdraw' } as const;
export type DecisionKind = (typeof DecisionKind)[keyof typeof DecisionKind];

/** What the decision bar shows and enables for the caller. */
export type DecisionView = {
  canApprove: boolean;
  canReject: boolean;
  canWithdraw: boolean;
  notice: string | null;
};

/** Texts of the review page. */
export const ReviewText = {
  OwnRequest: 'You requested this deploy, so someone else has to approve it.',
  NotApprover: 'Only Approvers and Admins of this team (other than the requester) can decide.',
  NoApprover: 'Nobody else in this team can approve yet. Ask an admin to add an Approver.',
  Decided: 'This request is closed.',
  Approved: 'Approved — the new version is rolling out.',
  Rejected: 'Rejected — the draft stays editable.',
  Withdrawn: 'Request withdrawn.',
  Commented: 'Comment added',
} as const;

/** Decision states of the review page (server flags + local note rules); the server re-checks everything. */
export class DecisionModel {
  static view(detail: ApprovalDetail, note: string): DecisionView {
    const pending = ApprovalRules.isPending(detail.approval);
    return {
      canApprove: pending && detail.canDecide,
      canReject: pending && detail.canDecide && ApprovalRules.validRejectNote(note),
      canWithdraw: pending && detail.canWithdraw,
      notice: DecisionModel.notice(detail, pending),
    };
  }

  private static notice(detail: ApprovalDetail, pending: boolean): string | null {
    if (!pending) return ReviewText.Decided;
    if (detail.eligibleApprovers === 0) return ReviewText.NoApprover;
    if (detail.canDecide) return null;
    return detail.canWithdraw ? ReviewText.OwnRequest : ReviewText.NotApprover;
  }

  /** Toast after a decision. */
  static done(kind: DecisionKind): string {
    if (kind === DecisionKind.Approve) return ReviewText.Approved;
    if (kind === DecisionKind.Reject) return ReviewText.Rejected;
    return ReviewText.Withdrawn;
  }
}
