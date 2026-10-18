import { AccessPolicy, DeployMode, EnvTag, Permission, type Access } from '../domain';

/** What the deploy button needs to know: who deploys, to which team, which destinations, under which policy. */
export type DeployContext = {
  access: Access | null;
  teamId: string | null;
  destinations: readonly string[];
  requireProdApproval: boolean;
};

/**
 * Client mirror of the Go `TeamProductionPolicy` (flow 5.10, golden table testdata/approvals/gate_table.json):
 * non-prod needs deploy_nonprod; prod goes direct only without the team policy and with deploy_prod;
 * otherwise writers request approval. The server's 202 / 403 stays authoritative.
 */
export class DeployModeResolver {
  static resolve(ctx: DeployContext): DeployMode {
    const can = (p: Permission) => AccessPolicy.can(ctx.access, p, ctx.teamId);
    if (!ctx.destinations.includes(EnvTag.Prod)) return can(Permission.RouteDeployNonProd) ? DeployMode.Deploy : DeployMode.Disabled;
    if (!ctx.requireProdApproval && can(Permission.RouteDeployProd)) return DeployMode.Deploy;
    return can(Permission.RouteWrite) ? DeployMode.RequestApproval : DeployMode.Disabled;
  }

  /** Button label of a mode; `deployLabel` is the screen's own wording for a direct deploy. */
  static label(mode: DeployMode, deployLabel: string): string {
    return DeployModeLabel[mode] ?? deployLabel;
  }
}

/** Labels of the non-default modes (Deploy keeps the screen's wording). */
export const DeployModeLabel: Partial<Record<DeployMode, string>> = {
  [DeployMode.RequestApproval]: 'Request approval',
  [DeployMode.Disabled]: "You can't deploy this route",
};
