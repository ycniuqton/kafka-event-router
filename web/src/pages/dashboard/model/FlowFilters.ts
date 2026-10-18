import { EnvMeta, EnvTag, type Connection, type FlowMapQuery, type Route } from '../../../domain';

/** Keys of the flow-map filter dimensions (spec §4: by connection, team, tag). */
export const FlowFilterKey = { Connection: 'connection', Team: 'team', Tag: 'tag' } as const;
export type FlowFilterKey = (typeof FlowFilterKey)[keyof typeof FlowFilterKey];

/** Sentinel value meaning "no filter on this dimension". */
export const FlowFilterAll = '' as const;

export type FlowFilterValues = Record<FlowFilterKey, string>;
export type FlowFilterOption = { value: string; label: string };
export type FlowFilterContext = { routes: Route[]; connections: Connection[] };

/** One way of narrowing the routes drawn on the flow map. */
export abstract class FlowFilterDimension {
  abstract readonly key: FlowFilterKey;
  abstract readonly allLabel: string;
  abstract options(ctx: FlowFilterContext): FlowFilterOption[];
}

/** Routes reading from or writing to a connection. */
class ConnectionDimension extends FlowFilterDimension {
  readonly key = FlowFilterKey.Connection;
  readonly allLabel = 'All connections';

  options(ctx: FlowFilterContext): FlowFilterOption[] {
    return ctx.connections.map((c) => ({ value: c.id, label: c.name || c.id }));
  }
}

/** Routes owned by a team. */
class TeamDimension extends FlowFilterDimension {
  readonly key = FlowFilterKey.Team;
  readonly allLabel = 'All teams';

  options(ctx: FlowFilterContext): FlowFilterOption[] {
    const teams = [...new Set(ctx.routes.map((r) => r.owner))].sort();
    return teams.map((t) => ({ value: t, label: t }));
  }
}

/** Routes touching a connection with an environment tag (prod / staging / dev) or tagged with it. */
class TagDimension extends FlowFilterDimension {
  readonly key = FlowFilterKey.Tag;
  readonly allLabel = 'All tags';

  options(): FlowFilterOption[] {
    return Object.values(EnvTag).map((t) => ({ value: t, label: EnvMeta[t].label }));
  }
}

/** Owns the configured filter dimensions and turns the chosen values into the server query. */
export class FlowFilterManager {
  static readonly dimensions: FlowFilterDimension[] = [new ConnectionDimension(), new TeamDimension(), new TagDimension()];

  static empty(): FlowFilterValues {
    return { [FlowFilterKey.Connection]: FlowFilterAll, [FlowFilterKey.Team]: FlowFilterAll, [FlowFilterKey.Tag]: FlowFilterAll };
  }

  /** The `GET /flow-map` query of the chosen values (the server filters; empty = all). */
  static query(values: FlowFilterValues): FlowMapQuery {
    return { connection: values[FlowFilterKey.Connection], team: values[FlowFilterKey.Team], tag: values[FlowFilterKey.Tag] };
  }

  static isFiltered(values: FlowFilterValues): boolean {
    return Object.values(values).some((v) => v !== FlowFilterAll);
  }
}
