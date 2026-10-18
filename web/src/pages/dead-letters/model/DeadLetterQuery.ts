import { DeadLetterKind, DeadLetterTimeRange, DeadLetterTimeRanges, type DeadLetterFilter } from '../../../domain';

/** Query-string keys of the dead-letter browser: `?route=&type=<kind>,<kind>&range=24h&step=&ver=&q=&resolved=1&sel=<id>`. */
export const DeadLetterQuery = {
  Route: 'route',
  Selected: 'sel',
  Kind: 'type',
  Range: 'range',
  Step: 'step',
  Version: 'ver',
  Text: 'q',
  Resolved: 'resolved',
} as const;

/** Value of the route filter that shows every route. */
export const AllRoutesFilter = '';

/** Value of the step / version filters that shows everything. */
export const AnyValueFilter = '';

/** Value of `?resolved=` that shows resolved rows too. */
export const ResolvedShown = '1';

/** Separator of the multi-value `type` parameter. */
export const KindSeparator = ',';

/** The browser filter as kept in the URL; `fixedRouteId` (route tab) wins over `?route=`. */
export type DeadLetterView = {
  routeId?: string;
  kinds: DeadLetterKind[];
  range: DeadLetterTimeRange;
  stepId?: string;
  versionNo?: number;
  text?: string;
  includeResolved: boolean;
};

/** Reads and writes the browser filter from / to URL params. */
export class DeadLetterViewParams {
  static parse(params: URLSearchParams, fixedRouteId?: string): DeadLetterView {
    const known = Object.values(DeadLetterKind) as string[];
    const kinds = (params.get(DeadLetterQuery.Kind) ?? '').split(KindSeparator).filter((k) => known.includes(k)) as DeadLetterKind[];
    const routeId = fixedRouteId ?? (params.get(DeadLetterQuery.Route) || undefined);
    const version = Number(params.get(DeadLetterQuery.Version));
    return {
      routeId,
      kinds,
      range: DeadLetterTimeRanges.parse(params.get(DeadLetterQuery.Range)),
      stepId: params.get(DeadLetterQuery.Step) || undefined,
      versionNo: Number.isInteger(version) && version > 0 ? version : undefined,
      text: params.get(DeadLetterQuery.Text)?.trim() || undefined,
      includeResolved: params.get(DeadLetterQuery.Resolved) === ResolvedShown,
    };
  }

  /** Server filter of a view; the time window ends now. */
  static filter(view: DeadLetterView, now: Date): DeadLetterFilter {
    const { routeId, kinds, stepId, versionNo, text, includeResolved } = view;
    return { routeId, kinds, since: DeadLetterTimeRanges.since(view.range, now), stepId, versionNo, text, includeResolved };
  }

  /** Stable key of a view (reload trigger). */
  static key(view: DeadLetterView): string {
    const p2 = [view.stepId ?? AnyValueFilter, view.versionNo ?? AnyValueFilter, view.text ?? AnyValueFilter, view.includeResolved];
    return [view.routeId ?? AllRoutesFilter, view.kinds.join(KindSeparator), view.range, ...p2].join('|');
  }

  static isFiltered(view: DeadLetterView, fixedRouteId?: string): boolean {
    const p2 = !!view.stepId || !!view.versionNo || !!view.text;
    return view.kinds.length > 0 || view.range !== DeadLetterTimeRange.All || (!fixedRouteId && !!view.routeId) || p2;
  }

  static toggleKind(view: DeadLetterView, kind: DeadLetterKind): string | null {
    const next = view.kinds.includes(kind) ? view.kinds.filter((k) => k !== kind) : [...view.kinds, kind];
    return next.length ? next.join(KindSeparator) : null;
  }

  /** URL params cleared by "Clear filters" (the route stays on the route tab). */
  static cleared(fixedRouteId?: string): Record<string, null> {
    const keys = [DeadLetterQuery.Kind, DeadLetterQuery.Range, DeadLetterQuery.Step, DeadLetterQuery.Version, DeadLetterQuery.Text];
    return Object.fromEntries([...keys, ...(fixedRouteId ? [] : [DeadLetterQuery.Route])].map((k) => [k, null]));
  }
}
