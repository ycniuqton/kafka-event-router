import type { BuilderStep, RouteDetailTab, RouteStatus, RouteTemplateId } from '../domain';

/** Query-string keys understood by the screens. */
export const QueryParam = {
  Search: 'q',
  Status: 'status',
  Tab: 'tab',
  Template: 'template',
  Step: 'step',
  Sample: 'sample',
  Session: 'session',
  Test: 'test',
  Connection: 'connection',
  Tag: 'tag',
  Owner: 'owner',
} as const;

/** Router path patterns (relative to the shell root). */
export const RoutePattern = {
  Dashboard: '/',
  Routes: 'routes',
  RouteNew: 'routes/new',
  RouteDetail: 'routes/:id',
  RouteEdit: 'routes/:id/edit',
  Connections: 'connections',
  Library: 'library',
  DeadLetters: 'dead-letters',
  Audit: 'audit',
  Settings: 'settings',
  Approval: 'approvals/:id',
  NotFound: '*',
} as const;

type Query = Record<string, string | number | undefined | null>;

/** Builds every in-app URL. Never hand-write paths in screens — use these. */
export class AppPaths {
  static dashboard(): string {
    return '/';
  }

  static routes(opts: { q?: string; status?: RouteStatus | 'all' } = {}): string {
    return AppPaths.withQuery('/routes', { [QueryParam.Search]: opts.q, [QueryParam.Status]: opts.status });
  }

  static routeDetail(id: string, tab?: RouteDetailTab): string {
    return AppPaths.withQuery(`/routes/${encodeURIComponent(id)}`, { [QueryParam.Tab]: tab });
  }

  static routeNew(template?: RouteTemplateId): string {
    return AppPaths.withQuery('/routes/new', { [QueryParam.Template]: template });
  }

  /** Builder for an existing route; `sampleId` preselects a sample in the Transform Studio. */
  static routeEdit(id: string, step?: BuilderStep, sampleId?: number): string {
    return AppPaths.withQuery(`/routes/${encodeURIComponent(id)}/edit`, { [QueryParam.Step]: step, [QueryParam.Sample]: sampleId });
  }

  /** Connections list; `test` opens the test drawer for that connection. */
  static connections(opts: { test?: string } = {}): string {
    return AppPaths.withQuery('/connections', { [QueryParam.Test]: opts.test });
  }

  static library(): string {
    return '/library';
  }

  static deadLetters(): string {
    return '/dead-letters';
  }

  static audit(): string {
    return '/audit';
  }

  static settings(): string {
    return '/settings';
  }

  /** Review page of one production deploy request. */
  static approval(id: string): string {
    return `/approvals/${encodeURIComponent(id)}`;
  }

  private static withQuery(path: string, query: Query): string {
    const params = new URLSearchParams();
    Object.entries(query).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') params.set(k, String(v));
    });
    const qs = params.toString();
    return qs ? `${path}?${qs}` : path;
  }
}
