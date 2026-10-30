import { DeadLetterClasses, DeadLetterResolutionState, type DeadLetterErrorClass, DeadLetterSummaries, type DeadLetter, type DeadLetterFilter, type DeadLetterKindCount, type DeadLetterSelection, type DeadLetterSummary, type Route } from '../../../domain';
import type { DeadLetterView } from './DeadLetterQuery';

/** Route display names: the routes store first, then the DLQ summary, then the row's own name. */
export class DeadLetterRouteNames {
  private readonly routes: Route[];
  private readonly summary: DeadLetterSummary;

  constructor(routes: Route[], summary: DeadLetterSummary) {
    this.routes = routes;
    this.summary = summary;
  }

  of(routeId: string, fallback?: string): string {
    const stored = this.routes.find((r) => r.id === routeId)?.name;
    return stored || DeadLetterSummaries.route(this.summary, routeId)?.routeName || fallback || routeId;
  }

  /** Route filter options: routes with unresolved dead letters, plus the current filter. */
  options(current: string | undefined): { id: string; name: string }[] {
    const ids = this.summary.routes.map((r) => r.routeId);
    if (current && !ids.includes(current)) ids.push(current);
    return ids.map((id) => ({ id, name: this.of(id) })).sort((a, b) => a.name.localeCompare(b.name));
  }
}

/** Counts shown by the toolbar for the current view. */
export class DeadLetterViewCounts {
  static unresolved(summary: DeadLetterSummary, view: DeadLetterView): number {
    return view.routeId ? (DeadLetterSummaries.route(summary, view.routeId)?.unresolved ?? 0) : summary.unresolved;
  }

  /** Class chips: the server's class counts on the global view, else derived from the kind counts. */
  static classes(summary: DeadLetterSummary, view: DeadLetterView): Record<DeadLetterErrorClass, number> {
    const derived = DeadLetterClasses.counts(DeadLetterViewCounts.kinds(summary, view));
    return view.routeId || !summary.byClass ? derived : { ...derived, ...summary.byClass };
  }

  /** Kind chips: summary counts of the route (or all routes), plus selected kinds without unresolved events. */
  static kinds(summary: DeadLetterSummary, view: DeadLetterView): DeadLetterKindCount[] {
    const base = view.routeId ? (DeadLetterSummaries.route(summary, view.routeId)?.byKind ?? []) : summary.byKind;
    const missing = view.kinds.filter((k) => !base.some((c) => c.kind === k)).map((kind) => ({ kind, count: 0 }));
    return [...base, ...missing];
  }
}

/** What the toolbar's Export covers: the checked rows of one route, or every filtered event of the viewed route. */
export class DeadLetterExportTarget {
  static of(view: DeadLetterView, checked: DeadLetter[], filter: DeadLetterFilter): { routeId: string; selection: DeadLetterSelection } | null {
    const routeIds = [...new Set(checked.map((d) => d.routeId))];
    if (checked.length && routeIds.length === 1) return { routeId: routeIds[0], selection: { ids: checked.map((d) => d.id) } };
    if (checked.length || !view.routeId) return null;
    return { routeId: view.routeId, selection: { filter } };
  }
}

/** Options of the step and version filters, derived from the loaded rows plus the current values. */
export class DeadLetterFacetOptions {
  static steps(items: DeadLetter[], current?: string): { id: string; label: string }[] {
    const byId = new Map<string, string>();
    items.forEach((d) => byId.set(d.stepId ?? d.step, d.step));
    if (current && !byId.has(current)) byId.set(current, current);
    return [...byId].map(([id, label]) => ({ id, label })).sort((a, b) => a.label.localeCompare(b.label));
  }

  static versions(items: DeadLetter[], current?: number): number[] {
    const set = new Set(items.map((d) => d.versionNo).filter((v): v is number => !!v));
    if (current) set.add(current);
    return [...set].sort((a, b) => b - a);
  }
}

/** Which checked rows an action applies to. */
export class DeadLetterCheckedRows {
  static discarded(rows: DeadLetter[]): string[] {
    return rows.filter((d) => d.resolution?.state === DeadLetterResolutionState.Discarded).map((d) => d.id);
  }

  static unresolved(rows: DeadLetter[]): string[] {
    return rows.filter((d) => !d.resolution).map((d) => d.id);
  }
}
