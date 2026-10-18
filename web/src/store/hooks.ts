import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { services } from '../api';
import { DeadLetterSummaries, type ConnectionTestRun, type DeadLetterJob, type DeadLetterSummary, type Route, type RouteVersion, type UserSettings } from '../domain';
import type { AppState } from './AppStore';
import { actions, appStore, settingsStore } from './instance';

/**
 * Subscribes to a slice of the app store. The selector must return a stable reference
 * (a stored field) — derive filtered arrays with useMemo, not inside the selector.
 */
export function useAppSelector<T>(selector: (state: AppState) => T): T {
  return useSyncExternalStore(appStore.subscribe, () => selector(appStore.getState()));
}

export const useAppReady = () => useAppSelector((s) => s.ready);
export const useRoutes = () => useAppSelector((s) => s.routes);
export const useConnections = () => useAppSelector((s) => s.connections);
export const useActivity = () => useAppSelector((s) => s.activity);
export const useAlerts = () => useAppSelector((s) => s.alerts);
export const useAlertsUnread = () => useAppSelector((s) => s.alertsUnread);
export const usePushState = () => useAppSelector((s) => s.pushState);
/** Increments on every push frame that changed shared data (pages add it to their query deps). */
export const usePushSeq = () => useAppSelector((s) => s.pushSeq);
export const useCurrentUser = () => useAppSelector((s) => s.currentUser);
export const useToast = () => useAppSelector((s) => s.toast);
export const useNewRouteChooserOpen = () => useAppSelector((s) => s.newRouteChooserOpen);

export function useRoute(id: string | undefined): Route | undefined {
  const routes = useRoutes();
  return useMemo(() => routes.find((r) => r.id === id), [routes, id]);
}

/** Unresolved dead-letter counts of every readable route (refreshed after each dead-letter action). */
export const useDeadLetterSummary = (): DeadLetterSummary => useAppSelector((s) => s.deadLetterSummary);
/** Background dead-letter jobs started in this session (newest first), with live progress. */
export const useDeadLetterJobs = (): DeadLetterJob[] => useAppSelector((s) => s.deadLetterJobs);

/** Unresolved dead letters of one route, or of every route. */
export function useDeadLetterCount(routeId?: string): number {
  const summary = useDeadLetterSummary();
  return routeId ? (DeadLetterSummaries.route(summary, routeId)?.unresolved ?? 0) : summary.unresolved;
}

/** Latest progress of a connection test, if one ran in this session. */
export function useConnectionTest(connectionId: string | undefined): ConnectionTestRun | undefined {
  return useAppSelector((s) => (connectionId ? s.connectionTests[connectionId] : undefined));
}

/** Version history of a route; fetched on first use. */
export function useVersions(routeId: string | undefined): RouteVersion[] | undefined {
  const versions = useAppSelector((s) => (routeId ? s.versions[routeId] : undefined));
  useEffect(() => {
    if (routeId && !versions) void actions.routes.loadVersions(routeId);
  }, [routeId, versions]);
  return versions;
}

export function useSettings(): UserSettings {
  return useSyncExternalStore(settingsStore.subscribe, settingsStore.getState);
}

export const NowRefresh = { defaultMs: 30_000 } as const;

/** Current time from the active clock, refreshed periodically for relative labels. */
export function useNow(refreshMs: number = NowRefresh.defaultMs): Date {
  const [now, setNow] = useState(() => services.clock.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(services.clock.now()), refreshMs);
    return () => clearInterval(timer);
  }, [refreshMs]);
  return now;
}


export type QueryState<T> = { data: T | undefined; loading: boolean; error: Error | undefined; reload: () => void };

/**
 * Runs a one-off service call for page-local data (audit, library, samples…).
 * Re-runs when the JSON of `deps` changes or `reload()` is called.
 */
export function useServiceQuery<T>(load: () => Promise<T>, deps: readonly unknown[]): QueryState<T> {
  const [state, setState] = useState<{ data?: T; error?: Error; loading: boolean }>({ loading: true });
  const [nonce, setNonce] = useState(0);
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  });
  const key = JSON.stringify(deps);
  useEffect(() => {
    let alive = true;
    loadRef.current().then(
      (data) => alive && setState({ data, loading: false }),
      (error: Error) => alive && setState({ error, loading: false }),
    );
    return () => {
      alive = false;
    };
  }, [key, nonce]);
  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { data: state.data, loading: state.loading, error: state.error, reload };
}
