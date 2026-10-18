import type { Approval, ApprovalComment, ApprovalDetail, ApprovalFilter } from '../../../domain';
import { ApprovalsService } from '../../contracts';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import type { HttpContext } from '../core/HttpContext';
import { EntityTag } from '../members/MemberDtos';

/** REST paths of approvals (relative to `/api/v1`). */
export class ApprovalsEndpoint {
  static readonly approvals = '/approvals';

  static approval(id: string): string {
    return `${ApprovalsEndpoint.approvals}/${encodeURIComponent(id)}`;
  }

  static action(id: string, action: ApprovalAction): string {
    return `${ApprovalsEndpoint.approval(id)}/${action}`;
  }
}

/** Sub-resources of one approval. */
export const ApprovalAction = { Approve: 'approve', Reject: 'reject', Withdraw: 'withdraw', Comments: 'comments' } as const;
export type ApprovalAction = (typeof ApprovalAction)[keyof typeof ApprovalAction];

/** Approvals over `/approvals`: decisions carry If-Match of the request's row version. */
export class HttpApprovalsService extends ApprovalsService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  list(filter: ApprovalFilter): Promise<Approval[]> {
    const query = { status: filter.status, teamId: filter.teamId, routeId: filter.routeId, mine: filter.mine ? true : undefined };
    return new CursorIterator<Approval>(this.context.api, ApprovalsEndpoint.approvals, { query, limit: PageLimit.Max }).all();
  }

  get(id: string): Promise<ApprovalDetail> {
    return this.context.api.get<ApprovalDetail>(ApprovalsEndpoint.approval(id));
  }

  approve(approval: Approval, note: string): Promise<Approval> {
    return this.decide(approval, ApprovalAction.Approve, note);
  }

  reject(approval: Approval, note: string): Promise<Approval> {
    return this.decide(approval, ApprovalAction.Reject, note);
  }

  withdraw(approval: Approval): Promise<Approval> {
    return this.context.api.post<Approval>(ApprovalsEndpoint.action(approval.id, ApprovalAction.Withdraw));
  }

  comment(approvalId: string, body: string): Promise<ApprovalComment> {
    return this.context.api.post<ApprovalComment>(ApprovalsEndpoint.action(approvalId, ApprovalAction.Comments), { body: body.trim() });
  }

  private decide(approval: Approval, action: ApprovalAction, note: string): Promise<Approval> {
    return this.context.api.post<Approval>(ApprovalsEndpoint.action(approval.id, action), { note: note.trim() }, {
      ifMatch: EntityTag.of(approval.rowVersion),
    });
  }
}
