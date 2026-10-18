import { useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { QueryParam } from '../../../app/paths';
import { AnyOption, StatusFilterAll, StatusFilterOptions, type RouteListFilter, type StatusFilter } from './RouteListModel';

export type RouteListParams = RouteListFilter & {
  setQuery: (q: string) => void;
  setStatus: (s: StatusFilter) => void;
  setConnection: (id: string) => void;
  setTag: (tag: string) => void;
  setOwner: (owner: string) => void;
};

/** `?q=`, `?status=`, `?connection=`, `?tag=` and `?owner=` of the routes list; the URL is the single source of truth. */
export function useRouteListParams(): RouteListParams {
  const [params, setParams] = useSearchParams();
  const query = params.get(QueryParam.Search) ?? '';
  const status = StatusFilterOptions.parse(params.get(QueryParam.Status));
  const connectionId = params.get(QueryParam.Connection) ?? AnyOption;
  const tag = params.get(QueryParam.Tag) ?? AnyOption;
  const owner = params.get(QueryParam.Owner) ?? AnyOption;
  const update = useCallback(
    (key: string, value: string, empty: string) =>
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value === empty) next.delete(key);
          else next.set(key, value);
          return next;
        },
        { replace: true },
      ),
    [setParams],
  );
  const setQuery = useCallback((q: string) => update(QueryParam.Search, q, ''), [update]);
  const setStatus = useCallback((s: StatusFilter) => update(QueryParam.Status, s, StatusFilterAll), [update]);
  const setConnection = useCallback((id: string) => update(QueryParam.Connection, id, AnyOption), [update]);
  const setTag = useCallback((t: string) => update(QueryParam.Tag, t, AnyOption), [update]);
  const setOwner = useCallback((o: string) => update(QueryParam.Owner, o, AnyOption), [update]);
  return {
    query,
    status,
    connectionId,
    tag,
    owner,
    setQuery,
    setStatus,
    setConnection,
    setTag,
    setOwner,
  };
}
