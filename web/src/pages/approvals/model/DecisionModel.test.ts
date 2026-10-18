import { describe, expect, it } from 'vitest';
import { ApprovalStatus, type ApprovalDetail } from '../../../domain';
import { DecisionKind, DecisionModel, ReviewText } from './DecisionModel';

function detail(over: Partial<ApprovalDetail> = {}, status: ApprovalStatus = ApprovalStatus.Pending): ApprovalDetail {
  return {
    approval: {
      id: 'a', routeId: 'r', routeName: 'orders', teamId: 't', versionId: 'v', versionNo: 2, status, requestedBy: { id: 'u1', name: 'Hoa' },
      requestedAt: '2026-10-13T09:00:00Z', changeNote: '', decidedBy: null, decidedAt: null, decisionNote: '', destinations: [], rowVersion: 1,
    },
    requested: { no: 2, config: {} }, active: null, comments: [], canDecide: false, canWithdraw: false, eligibleApprovers: 1, ...over,
  };
}

describe('DecisionModel', () => {
  it('lets an approver approve, and reject only with a note', () => {
    expect(DecisionModel.view(detail({ canDecide: true }), '')).toEqual({ canApprove: true, canReject: false, canWithdraw: false, notice: null });
    expect(DecisionModel.view(detail({ canDecide: true }), 'too broad').canReject).toBe(true);
  });

  it('tells the requester someone else must approve and lets them withdraw', () => {
    const v = DecisionModel.view(detail({ canWithdraw: true }), 'x');
    expect(v).toEqual({ canApprove: false, canReject: false, canWithdraw: true, notice: ReviewText.OwnRequest });
  });

  it('explains when the caller may not decide or nobody can', () => {
    expect(DecisionModel.view(detail(), '').notice).toBe(ReviewText.NotApprover);
    expect(DecisionModel.view(detail({ eligibleApprovers: 0, canWithdraw: true }), '').notice).toBe(ReviewText.NoApprover);
  });

  it('closes every action once decided', () => {
    [ApprovalStatus.Rejected, ApprovalStatus.RollingOut, ApprovalStatus.Cancelled].forEach((s) => {
      const v = DecisionModel.view(detail({ canDecide: true, canWithdraw: true }, s), 'note');
      expect(v).toEqual({ canApprove: false, canReject: false, canWithdraw: false, notice: ReviewText.Decided });
    });
  });

  it('confirms each decision', () => {
    expect(DecisionModel.done(DecisionKind.Approve)).toBe(ReviewText.Approved);
    expect(DecisionModel.done(DecisionKind.Reject)).toBe(ReviewText.Rejected);
    expect(DecisionModel.done(DecisionKind.Withdraw)).toBe(ReviewText.Withdrawn);
  });
});
