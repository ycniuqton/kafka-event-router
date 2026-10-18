import type { ConnectionGrant, Team } from '../domain';
import { MockTeamId } from './members';

/** Settings → Teams (design teams) with ids, approval policy and versions as `GET /teams` returns them. */
export const TEAMS: Team[] = [
  { id: MockTeamId.Platform, name: 'Platform', requireProdApproval: true, memberCount: 1, createdAt: '2026-01-05T09:00:00Z', rowVersion: 1 },
  { id: MockTeamId.Payments, name: 'Payments', requireProdApproval: true, memberCount: 1, createdAt: '2026-01-05T09:00:00Z', rowVersion: 1 },
  { id: MockTeamId.Logistics, name: 'Logistics', requireProdApproval: false, memberCount: 1, createdAt: '2026-02-11T09:00:00Z', rowVersion: 1 },
  { id: MockTeamId.Leadership, name: 'Leadership', requireProdApproval: true, memberCount: 1, createdAt: '2026-03-02T09:00:00Z', rowVersion: 1 },
];

/** Connection grants: Logistics may use the Payments cluster. */
export const GRANTS: ConnectionGrant[] = [
  {
    teamId: MockTeamId.Logistics, connectionId: 'payments-prod', connectionName: 'payments-prod', ownerTeamId: MockTeamId.Payments,
    envTag: 'prod', grantedBy: 'user-quincy', createdAt: '2026-09-01T10:00:00Z',
  },
];
