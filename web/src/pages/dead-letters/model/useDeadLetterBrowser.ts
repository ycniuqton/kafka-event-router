import { useCallback, useEffect, useRef, useState } from 'react';
import { services } from '../../../api';
import {
  DeadLetterFileFormat,
  DeadLetterJobAction,
  DeadLetterJobs,
  DeadLetterReports,
  type DeadLetterActionReport,
  type DeadLetterApprovalRequest,
  type DeadLetterFilter,
  type DeadLetterJobOptions,
} from '../../../domain';
import { actions, useDeadLetterJobs } from '../../../store';
import { DeadLetterBrowserState } from './DeadLetterBrowserState';
import { DeadLetterViewParams, type DeadLetterView } from './DeadLetterQuery';

/** Error text of a failed page load. */
class LoadError {
  static of(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
}

type Holder = { key: string; state: DeadLetterBrowserState };

/** Pages through the dead letters of a view: first page on filter change or refresh, then "load more". */
function useDeadLetterPages(view: DeadLetterView, initialSelection: string | null) {
  const [nonce, setNonce] = useState(0);
  const key = `${DeadLetterViewParams.key(view)}#${nonce}`;
  const [holder, setHolder] = useState<Holder>(() => ({ key: '', state: DeadLetterBrowserState.initial(initialSelection) }));
  const state = holder.key === key ? holder.state : holder.state.reloading();
  const viewRef = useRef(view);
  const stateRef = useRef(state);
  const filterRef = useRef<DeadLetterFilter>(DeadLetterViewParams.filter(view, services.clock.now()));
  const generation = useRef(0);
  useEffect(() => {
    viewRef.current = view;
    stateRef.current = state;
  });
  const setState = useCallback((fn: (s: DeadLetterBrowserState) => DeadLetterBrowserState) => setHolder((h) => ({ ...h, state: fn(h.state) })), []);

  useEffect(() => {
    filterRef.current = DeadLetterViewParams.filter(viewRef.current, services.clock.now());
    const gen = ++generation.current;
    const settle = (fn: (s: DeadLetterBrowserState) => DeadLetterBrowserState) => gen === generation.current && setHolder((h) => ({ key, state: fn(h.state.reloading()) }));
    actions.deadLetters.page(filterRef.current).then(
      (page) => settle((s) => s.loaded(page, false)),
      (error: unknown) => settle((s) => s.failed(LoadError.of(error))),
    );
  }, [key]);

  const loadMore = useCallback(() => {
    const s = stateRef.current;
    if (!s.hasMore || s.loading) return;
    const gen = generation.current;
    setState((x) => x.loadingMore());
    actions.deadLetters.page(filterRef.current, s.nextCursor).then(
      (page) => gen === generation.current && setState((x) => x.loaded(page, true)),
      (error: unknown) => gen === generation.current && setState((x) => x.failed(LoadError.of(error))),
    );
  }, [setState]);

  const refresh = useCallback(() => {
    setNonce((n) => n + 1);
    void actions.deadLetters.refreshSummary();
  }, []);
  return { state, setState, filter: filterRef, loadMore, refresh };
}

/** Reloads the list when a background job of this session finishes. */
function useJobRefresh(refresh: () => void) {
  const jobs = useDeadLetterJobs();
  const finished = jobs.filter((j) => DeadLetterJobs.done(j)).length;
  const seen = useRef(finished);
  useEffect(() => {
    if (finished > seen.current) refresh();
    seen.current = finished;
  }, [finished, refresh]);
}

/** State and commands of the dead-letter browser (route tab and global screen). */
export function useDeadLetterBrowser(view: DeadLetterView, initialSelection: string | null) {
  const { state, setState, filter, loadMore, refresh } = useDeadLetterPages(view, initialSelection);
  const [report, setReport] = useState<DeadLetterActionReport | null>(null);
  const [format, setFormat] = useState<DeadLetterFileFormat>(DeadLetterFileFormat.Json);
  const [approval, setApproval] = useState<DeadLetterApprovalRequest[]>([]);
  useJobRefresh(refresh);
  const showResolved = view.includeResolved;

  const apply = useCallback(
    (r: DeadLetterActionReport | null) => {
      if (!r) return;
      if (showResolved) refresh();
      else setState((s) => s.without(DeadLetterReports.gone(r)).withFailures(r.failed).clearChecked());
      if (r.failed.length || r.deliveredAtLeastOnce) setReport(r);
      if (r.approvalRequired?.length) setApproval(r.approvalRequired);
    },
    [setState, refresh, showResolved],
  );

  return {
    state,
    report,
    closeReport: () => setReport(null),
    select: (id: string) => setState((s) => s.select(id)),
    toggle: (id: string) => setState((s) => s.toggle(id)),
    toggleAll: () => setState((s) => s.toggleAll()),
    loadMore,
    refresh,
    retry: async (ids: string[]) => apply(await actions.deadLetters.retry(ids)),
    discard: async (ids: string[]) => apply(await actions.deadLetters.discard(ids)),
    retryAll: async (routeId: string) => apply(await actions.deadLetters.retryMatching(routeId, filter.current)),
    restore: async (ids: string[]) => (await actions.deadLetters.restore(ids)) && refresh(),
    startJob: (routeId: string, action: DeadLetterJobAction, options?: DeadLetterJobOptions) => actions.deadLetters.startJob(routeId, action, filter.current, options),
    /** Events a sync retry could not send because a second person has to approve (strict 4-eyes). */
    approval,
    closeApproval: () => setApproval([]),
    requestApproval: async (note: string) => {
      const requests = approval;
      setApproval([]);
      await Promise.all(requests.map((a) => actions.deadLetters.startJob(a.routeId, DeadLetterJobAction.Retry, filter.current, { ids: a.ids, note })));
    },
    /** A replayed row leaves the list (the summary is refreshed by the action). */
    resolved: (id: string) => (showResolved ? refresh() : setState((s) => s.without([id]))),
    format,
    setFormat,
    filter,
  };
}

export type DeadLetterBrowserModel = ReturnType<typeof useDeadLetterBrowser>;
