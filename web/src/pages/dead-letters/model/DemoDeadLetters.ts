import { AppPaths } from '../../../app/paths';
import { ConnectionDisplay, DeadLetterSummaries, type Connection, type DeadLetterSummary, type Route } from '../../../domain';
import { DeadLetterQuery } from './DeadLetterQuery';

/** The demo workspace's seeded dead letters (FR-20): the route reading the in-memory demo cluster and its link. */
export class DemoDeadLetters {
  /** First route whose source is a demo connection, if the workspace has one. */
  static route(connections: Connection[], routes: Route[]): Route | undefined {
    const demo = new Set(connections.filter((c) => ConnectionDisplay.isDemo(c)).map((c) => c.id));
    return routes.find((r) => demo.has(r.source.connectionId));
  }

  /** Dead-letters screen filtered to the demo route. */
  static path(route: Route): string {
    return `${AppPaths.deadLetters()}?${new URLSearchParams({ [DeadLetterQuery.Route]: route.id })}`;
  }

  static label(route: Route, summary: DeadLetterSummary): string {
    const count = DeadLetterSummaries.route(summary, route.id)?.unresolved ?? 0;
    const events = count === 1 ? '1 failed event' : `${count || 'Some'} failed events`;
    return `${events} on ${route.name} — see how dead letters are fixed and retried`;
  }
}
