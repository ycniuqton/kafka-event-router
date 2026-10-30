import type { DeadLetterFilter, DeadLetterGroup } from '../../../domain';

/** What a bulk replay covers: one cause (filter by fingerprint) or explicit ids of one route. */
export type ReplayJobTarget = { routeId: string; title: string; subtitle: string; detail: string; count: number; filter: DeadLetterFilter; ids?: string[] };

/** Replay target of a cause group. */
export class ReplayTargets {
  static group(g: DeadLetterGroup): ReplayJobTarget {
    return { routeId: g.routeId, title: 'Replay this cause', subtitle: `${g.routeName} · ${g.step}`, detail: g.pattern, count: g.count, filter: { routeId: g.routeId, kinds: [], fingerprint: g.fingerprint } };
  }

  static selection(routeId: string, routeName: string, ids: string[], filter: DeadLetterFilter): ReplayJobTarget {
    return { routeId, title: 'Replay the selected events', subtitle: routeName, detail: `${ids.length} selected events`, count: ids.length, filter, ids };
  }
}
