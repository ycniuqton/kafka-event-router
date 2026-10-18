import { describe, expect, it } from 'vitest';
import table from '../../../testdata/approvals/gate_table.json';
import { DeployMode, EnvTag, RoleMatrix, type Access, type RoleKey } from '../domain';
import { DeployModeResolver } from './DeployModeResolver';

const team = 'team-a';
const WorkspaceAdminRow = 'workspace_admin';

/** Gate outcome (Go) → deploy mode (web). */
const OutcomeMode: Record<string, DeployMode> = {
  deploy_now: DeployMode.Deploy,
  require_approval: DeployMode.RequestApproval,
  deny: DeployMode.Disabled,
};

function accessOf(role: string): Access {
  if (role === WorkspaceAdminRow) return RoleMatrix.accessFor(true, {});
  return RoleMatrix.accessFor(false, { [team]: role as RoleKey });
}

describe('DeployModeResolver', () => {
  it('mirrors the Go deploy gate table', () => {
    table.cases.forEach((c) => {
      const mode = DeployModeResolver.resolve({
        access: accessOf(c.role),
        teamId: team,
        destinations: [c.prod ? EnvTag.Prod : EnvTag.Dev],
        requireProdApproval: c.requireProdApproval,
      });
      expect({ ...c, mode }).toEqual({ ...c, mode: OutcomeMode[c.outcome] });
    });
  });

  it('denies until access is loaded and labels each mode', () => {
    const mode = DeployModeResolver.resolve({ access: null, teamId: team, destinations: [EnvTag.Prod], requireProdApproval: true });
    expect(mode).toBe(DeployMode.Disabled);
    expect(DeployModeResolver.label(DeployMode.RequestApproval, 'Deploy route')).toBe('Request approval');
    expect(DeployModeResolver.label(DeployMode.Deploy, 'Deploy route')).toBe('Deploy route');
  });
});
