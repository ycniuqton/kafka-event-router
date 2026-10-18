import { ApprovalStatus, type Approval, type ApprovalComment } from '../domain';
import { MockTeamId } from './members';

/** Approval requests (design §11.2): one waiting for Minh, one rejected with a note. */
export const APPROVALS: Approval[] = [
  {
    id: 'appr-1', routeId: 'orders-to-partner', routeName: 'orders-to-partner', teamId: MockTeamId.Payments, versionId: 'v-13',
    versionNo: 13, status: ApprovalStatus.Pending, requestedBy: { id: 'user-hoa', name: 'Hoa L.' }, requestedAt: '2026-10-13T09:12:00Z',
    changeNote: 'Mask card numbers before the partner sees them', decidedBy: null, decidedAt: null, decisionNote: '',
    destinations: [{ connectionId: 'partner-kafka', name: 'partner-kafka', envTag: 'prod' }], rowVersion: 1,
  },
  {
    id: 'appr-2', routeId: 'payments-audit', routeName: 'payments-audit', teamId: MockTeamId.Payments, versionId: 'v-7', versionNo: 7,
    status: ApprovalStatus.Rejected, requestedBy: { id: 'user-quincy', name: 'Quincy N.' }, requestedAt: '2026-10-12T15:40:00Z',
    changeNote: 'Send every payment event', decidedBy: { id: 'user-minh', name: 'Minh N.' }, decidedAt: '2026-10-12T16:05:00Z',
    decisionNote: 'Keep the amount filter — the audit topic is not sized for all events.',
    destinations: [{ connectionId: 'payments-prod', name: 'payments-prod', envTag: 'prod' }], rowVersion: 2,
  },
];

/** Review comments keyed by approval id. */
export const APPROVAL_COMMENTS: Record<string, ApprovalComment[]> = {
  'appr-1': [
    { id: 'c-1', approvalId: 'appr-1', author: { id: 'user-hoa', name: 'Hoa L.' }, body: 'Partner asked for this before Friday.', createdAt: '2026-10-13T09:13:00Z' },
  ],
};

/** Configurations shown in the mock review diff (requested vs active). */
export const APPROVAL_CONFIGS: Record<string, { requested: unknown; active: unknown }> = {
  'appr-1': {
    requested: { name: 'orders-to-partner', branches: [{ id: 'main', steps: [{ kind: 'mask', field: 'value.card' }], destination: { topic: 'orders-public' } }] },
    active: { name: 'orders-to-partner', branches: [{ id: 'main', steps: [], destination: { topic: 'orders-public' } }] },
  },
};
