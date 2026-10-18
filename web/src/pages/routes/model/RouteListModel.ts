import { Format } from '../../../components/ui/Format';
import { LagHealth, RouteEndpointText, RouteStatus, RouteStatusMeta, Tone, TransformModeMeta, type Route } from '../../../domain';

/** Layout of the routes list: dense table or cards. */
export const RoutesView = { Table: 'table', Cards: 'cards' } as const;
export type RoutesView = (typeof RoutesView)[keyof typeof RoutesView];

/** Value of the `?status=` filter: one route status or every route. */
export const StatusFilterAll = 'all';
export type StatusFilter = RouteStatus | typeof StatusFilterAll;

/** Status chips of the toolbar, in the design's order. */
export class StatusFilterOptions {
  static readonly chips: readonly StatusFilter[] = [
    StatusFilterAll,
    RouteStatus.Running,
    RouteStatus.Paused,
    RouteStatus.Error,
    RouteStatus.Draft,
    RouteStatus.Archived,
  ];

  static readonly allLabel = 'All';

  static label(filter: StatusFilter): string {
    return filter === StatusFilterAll ? StatusFilterOptions.allLabel : RouteStatusMeta[filter].label;
  }

  static parse(raw: string | null): StatusFilter {
    return StatusFilterOptions.chips.find((c) => c === raw) ?? StatusFilterAll;
  }

  /** "All" counts live routes; archived ones only count for their own chip. */
  static count(routes: Route[], filter: StatusFilter): number {
    if (filter === StatusFilterAll) return routes.filter((r) => r.status !== RouteStatus.Archived).length;
    return routes.filter((r) => r.status === filter).length;
  }
}

/** Every filter of the list: free text, status chip, connection (source or destination), tag and owner team. */
export type RouteListFilter = {
  query: string;
  status: StatusFilter;
  connectionId: string;
  tag: string;
  owner: string;
};

/** Value of the connection and tag dropdowns meaning "any". */
export const AnyOption = '';

/** Search and filtering of the routes list (name, connection names, topics, owner, tags). */
export class RouteListQuery {
  static apply(routes: Route[], filter: RouteListFilter): Route[] {
    const needle = filter.query.trim().toLowerCase();
    return routes.filter(
      (r) =>
        RouteListQuery.matchesStatus(r, filter.status) &&
        RouteListQuery.matchesConnection(r, filter.connectionId) &&
        RouteListQuery.matchesTag(r, filter.tag) &&
        RouteListQuery.matchesOwner(r, filter.owner) &&
        RouteListQuery.matchesText(r, needle),
    );
  }

  private static matchesStatus(route: Route, status: StatusFilter): boolean {
    return status === StatusFilterAll ? route.status !== RouteStatus.Archived : route.status === status;
  }

  private static matchesConnection(route: Route, connectionId: string): boolean {
    return connectionId === AnyOption || route.source.connectionId === connectionId || route.destination.connectionId === connectionId;
  }

  private static matchesTag(route: Route, tag: string): boolean {
    return tag === AnyOption || (route.tags ?? []).includes(tag);
  }

  private static matchesOwner(route: Route, owner: string): boolean {
    return owner === AnyOption || route.owner === owner;
  }

  private static matchesText(route: Route, needle: string): boolean {
    if (!needle) return true;
    const haystack = [route.name, RouteEndpointText.source(route), RouteEndpointText.destination(route), route.owner, (route.tags ?? []).join(' ')].join(' ');
    return haystack.toLowerCase().includes(needle);
  }
}

/** Connection and tag choices of the toolbar dropdowns, derived from the loaded routes. */
export class RouteFacets {
  /** Connections used by any route, as id → display name, sorted by name. */
  static connections(routes: Route[]): { id: string; name: string }[] {
    const byId = new Map<string, string>();
    routes.forEach((r) => {
      byId.set(r.source.connectionId, RouteEndpointText.sourceConnection(r));
      byId.set(r.destination.connectionId, RouteEndpointText.destinationConnection(r));
    });
    byId.delete(AnyOption);
    return [...byId].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
  }

  /** Owner teams of the loaded routes, sorted. */
  static owners(routes: Route[]): string[] {
    return [...new Set(routes.map((r) => r.owner).filter(Boolean))].sort();
  }

  static tags(routes: Route[]): string[] {
    return [...new Set(routes.flatMap((r) => r.tags ?? []))].sort();
  }
}

/** Display values of one route row or card (design `routeVM`). */
export class RouteRowModel {
  static readonly sparkSeedModulo = 17;
  static readonly topicJoiner = ' + ';

  readonly route: Route;

  constructor(route: Route) {
    this.route = route;
  }

  get topicText(): string {
    return this.route.source.topics.join(RouteRowModel.topicJoiner);
  }

  get sourceText(): string {
    return RouteEndpointText.source(this.route);
  }

  get destinationText(): string {
    return RouteEndpointText.destination(this.route);
  }

  get transformLabel(): string {
    return TransformModeMeta[this.route.transformMode].short;
  }

  get transformTone(): Tone {
    return TransformModeMeta[this.route.transformMode].tone;
  }

  get tags(): string[] {
    return this.route.tags ?? [];
  }

  get hasMetrics(): boolean {
    return !this.route.metrics.unavailable;
  }

  get throughputText(): string {
    return this.route.status === RouteStatus.Running && this.hasMetrics ? Format.rate(this.route.metrics.eventsPerSec) : Format.placeholder;
  }

  get matchText(): string {
    return this.route.status === RouteStatus.Draft || !this.hasMetrics ? Format.placeholder : Format.percent(this.route.metrics.matchPct);
  }

  get lagText(): string {
    return this.hasMetrics ? Format.number(this.route.metrics.lag) : Format.placeholder;
  }

  get lagTone(): Tone {
    return this.hasMetrics ? LagHealth.routeTone(this.route.metrics.lag) : Tone.Muted;
  }

  get sparkValues(): number[] | undefined {
    return this.route.metrics.sparkline;
  }

  get sparkSeed(): number {
    return (this.route.metrics.eventsPerSec % RouteRowModel.sparkSeedModulo) + this.route.id.length;
  }

  get sparkFlat(): boolean {
    return this.route.status !== RouteStatus.Running || !this.route.metrics.eventsPerSec;
  }

  get sparkTone(): Tone {
    return this.route.status === RouteStatus.Error ? Tone.Err : Tone.Accent;
  }
}

/** Toast copy of the list's bulk actions. */
export class RoutesCopy {
  static tagged(tag: string, count: number): string {
    return `Added tag “${tag}” to ${count} routes`;
  }
}

/** Viewport width below which the list switches to cards (phones). */
export const NarrowViewport = { query: '(max-width: 760px)' } as const;
