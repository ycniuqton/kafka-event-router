import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { services } from '../../../api';
import type { AuditEntry, AuditQuery } from '../../../domain';
import { AuditFilter } from './AuditFilter';

/** The loaded audit rows, the next cursor and the load state of the current query. */
export type AuditLogState = {
  entries: AuditEntry[];
  loading: boolean;
  error: Error | undefined;
  hasMore: boolean;
  loadMore: () => void;
};

/** Rows loaded for one query key; `pending` is true while a "load more" page is in flight. */
type Loaded = { key: string; entries: AuditEntry[]; cursor: string | null; pending: boolean; error?: Error };

/**
 * Loads the first page whenever the (debounced) query changes and appends pages on `loadMore`.
 * Results of an outdated query are dropped; while the first page loads the previous rows are hidden.
 */
export function useAuditLog(query: AuditQuery): AuditLogState {
  const debounced = useDebouncedQuery(query);
  const key = JSON.stringify(debounced);
  const [state, setState] = useState<Loaded | null>(null);
  const latest = useRef(key);
  const load = useCallback(
    (cursor: string | null, previous: AuditEntry[]) => {
      const keep = (next: Loaded) => setState((s) => (latest.current === next.key ? next : s));
      return services.audit.query(debounced, cursor).then(
        (page) => keep({ key, entries: [...previous, ...page.items], cursor: page.nextCursor, pending: false }),
        (error: Error) => keep({ key, entries: previous, cursor, pending: false, error }),
      );
    },
    [debounced, key],
  );
  useEffect(() => {
    latest.current = key;
    void load(null, []);
  }, [key, load]);
  const current = state?.key === key ? state : null;
  const loadMore = useCallback(() => {
    if (!current?.cursor || current.pending) return;
    setState({ ...current, pending: true });
    void load(current.cursor, current.entries);
  }, [current, load]);
  return {
    entries: current?.entries ?? [],
    loading: !current || current.pending,
    error: current?.error,
    hasMore: !!current?.cursor,
    loadMore,
  };
}

/** Text changes settle for `AuditFilter.textDebounceMs`; other filters apply at once. */
function useDebouncedQuery(query: AuditQuery): AuditQuery {
  const [text, setText] = useState(query.text);
  useEffect(() => {
    const timer = setTimeout(() => setText(query.text), AuditFilter.textDebounceMs);
    return () => clearTimeout(timer);
  }, [query.text]);
  const rest = JSON.stringify({ ...query, text: '' });
  return useMemo(() => ({ ...(JSON.parse(rest) as AuditQuery), text }), [text, rest]);
}
