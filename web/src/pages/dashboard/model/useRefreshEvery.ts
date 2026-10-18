import { useEffect, useRef } from 'react';
import { PushState } from '../../../domain';
import { usePushSeq, usePushState } from '../../../store';

/** Dashboard refresh cadence: push frames refresh at once; polling every 60 s while push is live, 15 s otherwise. */
export const DashboardRefresh = { intervalMs: 15_000, pushIntervalMs: 60_000 } as const;

/** Calls `reload` every `intervalMs` while the component is mounted. */
export function useRefreshEvery(reload: () => void, intervalMs: number = DashboardRefresh.intervalMs): void {
  useEffect(() => {
    const timer = setInterval(reload, intervalMs);
    return () => clearInterval(timer);
  }, [reload, intervalMs]);
}

/** Push-driven refresh: reloads when a push frame changed shared data, with a fallback poll. */
export function useLiveRefresh(reload: () => void): void {
  const live = usePushState() === PushState.Live;
  const seq = usePushSeq();
  const seen = useRef(seq);
  useEffect(() => {
    if (seen.current === seq) return;
    seen.current = seq;
    reload();
  }, [seq, reload]);
  useRefreshEvery(reload, live ? DashboardRefresh.pushIntervalMs : DashboardRefresh.intervalMs);
}
