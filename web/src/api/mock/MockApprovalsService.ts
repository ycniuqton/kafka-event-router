import { ApprovalRules, ApprovalStatus, CancelReason, type Approval, type ApprovalComment, type ApprovalDetail, type ApprovalFilter } from '../../domain';
import { APPROVAL_COMMENTS, APPROVAL_CONFIGS, APPROVALS, CURRENT_USER, CURRENT_USER_ID } from '../../mocks';
import { ApprovalsService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Messages of the mock approval rules (same wording as the API problems). */
export const MockApprovalsText = {
  SelfApproval: 'You requested this deploy, so someone else has to approve it.',
  Decided: 'This request was already decided. Reload to see the decision.',
  NoteRequired: 'Tell the requester why you reject this deploy.',
  InvalidComment: 'Write a comment of 1 to 4000 characters.',
  NotRequester: 'Only the person who requested this deploy (or a workspace admin) can withdraw it.',
  NotFound: 'This approval request does not exist. It may have been deleted.',
} as const;

/** In-memory approvals enforcing strict 4-eyes, one decision per request, notes and comment limits. */
export class MockApprovalsService extends ApprovalsService {
  private readonly backend: MockBackend;
  private items: Approval[] = structuredClone(APPROVALS);
  private comments: Record<string, ApprovalComment[]> = structuredClone(APPROVAL_COMMENTS);

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(filter: ApprovalFilter): Promise<Approval[]> {
    const rows = this.items.filter(
      (a) => (!filter.status || a.status === filter.status) && (!filter.routeId || a.routeId === filter.routeId)
        && (!filter.teamId || a.teamId === filter.teamId) && (!filter.mine || a.requestedBy?.id === CURRENT_USER_ID),
    );
    return this.backend.reply(rows);
  }

  get(id: string): Promise<ApprovalDetail> {
    const a = this.items.find((x) => x.id === id);
    if (!a) return this.backend.fail(MockApprovalsText.NotFound);
    const configs = APPROVAL_CONFIGS[id];
    const mine = a.requestedBy?.id === CURRENT_USER_ID;
    return this.backend.reply({
      approval: a, requested: { no: a.versionNo, config: configs?.requested ?? {} },
      active: configs ? { no: a.versionNo - 1, config: configs.active } : null, comments: this.comments[id] ?? [],
      canDecide: ApprovalRules.isPending(a) && !mine, canWithdraw: ApprovalRules.isPending(a), eligibleApprovers: 2,
    });
  }

  approve(approval: Approval, note: string): Promise<Approval> {
    return this.decide(approval, ApprovalStatus.RollingOut, note);
  }

  reject(approval: Approval, note: string): Promise<Approval> {
    if (!ApprovalRules.validRejectNote(note)) return this.backend.fail(MockApprovalsText.NoteRequired);
    return this.decide(approval, ApprovalStatus.Rejected, note);
  }

  withdraw(approval: Approval): Promise<Approval> {
    const row = this.items.find((a) => a.id === approval.id);
    if (!row) return this.backend.fail(MockApprovalsText.NotFound);
    if (!ApprovalRules.isPending(row)) return this.backend.fail(MockApprovalsText.Decided);
    Object.assign(row, { status: ApprovalStatus.Cancelled, cancelReason: CancelReason.Withdrawn, decidedAt: this.backend.nowIso(), rowVersion: row.rowVersion + 1 });
    return this.backend.reply(row);
  }

  comment(approvalId: string, body: string): Promise<ApprovalComment> {
    if (!ApprovalRules.validComment(body)) return this.backend.fail(MockApprovalsText.InvalidComment);
    const c: ApprovalComment = { id: `c-${Date.now()}`, approvalId, author: { id: CURRENT_USER_ID, name: CURRENT_USER.name }, body: body.trim(),
      createdAt: this.backend.nowIso() };
    this.comments[approvalId] = [...(this.comments[approvalId] ?? []), c];
    return this.backend.reply(c);
  }

  private decide(approval: Approval, status: ApprovalStatus, note: string): Promise<Approval> {
    const row = this.items.find((a) => a.id === approval.id);
    if (!row) return this.backend.fail(MockApprovalsText.NotFound);
    if (!ApprovalRules.isPending(row) || row.rowVersion !== approval.rowVersion) return this.backend.fail(MockApprovalsText.Decided);
    if (row.requestedBy?.id === CURRENT_USER_ID) return this.backend.fail(MockApprovalsText.SelfApproval);
    Object.assign(row, { status, decisionNote: note.trim(), decidedBy: { id: CURRENT_USER_ID, name: CURRENT_USER.name }, decidedAt: this.backend.nowIso(),
      rowVersion: row.rowVersion + 1 });
    return this.backend.reply(row);
  }
}
