import { useMemo } from 'react';
import { services } from '../../../api';
import { RouteStatus, type Route } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { RouteListQuery, RouteRowModel, type RouteListFilter } from './RouteListModel';

/** Rows of the routes list: live routes from the store, archived ones fetched for the "Archived" chip. */
export function useRoutesListData(routes: Route[], filter: RouteListFilter) {
  const archived = useServiceQuery(() => services.routes.listArchived(), [routes.length]);
  const { query, status, connectionId, tag, owner } = filter;
  const pool = useMemo(() => (status === RouteStatus.Archived ? (archived.data ?? []) : routes), [status, archived.data, routes]);
  const counted = useMemo(() => {
    const live = routes.filter((r) => r.status !== RouteStatus.Archived);
    return [...live, ...(archived.data ?? [])];
  }, [routes, archived.data]);
  const rows = useMemo(
    () =>
      RouteListQuery.apply(pool, {
        query,
        status,
        connectionId,
        tag,
        owner,
      }).map((r) => new RouteRowModel(r)),
    [pool, query, status, connectionId, tag, owner],
  );
  return { rows, pool, counted, reloadArchived: archived.reload };
}
