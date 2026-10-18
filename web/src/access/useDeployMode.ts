import { useMemo } from 'react';
import { services } from '../api';
import { BranchSet, TeamRules, type DeployMode, type RouteDraft } from '../domain';
import { useConnections, useRoute, useServiceQuery } from '../store';
import { DeployModeResolver } from './DeployModeResolver';
import { useAccess } from './useCan';

/** Deploy mode of a builder draft: its route's team (or the caller's only team for new routes) and destination env. */
export function useDeployMode(draft: RouteDraft): DeployMode {
  const access = useAccess();
  const route = useRoute(draft.routeId ?? undefined);
  const connections = useConnections();
  const { data: teams } = useServiceQuery(() => services.teams.list(), []);
  return useMemo(() => {
    const ownTeams = Object.keys(access?.teams ?? {});
    const team = TeamRules.find(teams ?? [], route?.owner ?? (ownTeams.length === 1 ? ownTeams[0] : null));
    const envs = BranchSet.all(draft).map((b) => connections.find((c) => c.id === b.destination.connectionId)?.env);
    return DeployModeResolver.resolve({
      access, teamId: team?.id ?? null, destinations: envs.filter((e) => e !== undefined), requireProdApproval: team?.requireProdApproval ?? true,
    });
  }, [access, route?.owner, teams, connections, draft]);
}
