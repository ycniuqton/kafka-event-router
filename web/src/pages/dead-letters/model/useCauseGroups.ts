import { useCallback, useEffect, useRef, useState } from 'react';
import { services } from '../../../api';
import { DeadLetterGroups, type DeadLetterGroupPage } from '../../../domain';
import { actions } from '../../../store';
import { DeadLetterViewParams, type DeadLetterView } from './DeadLetterQuery';

/** Load state of the Causes view. */
export type CauseGroupsState = { page: DeadLetterGroupPage; loading: boolean; error: string | null };

/** Loads the cause groups of a view (fingerprint ignored: groups are what you pick a fingerprint from). */
export function useCauseGroups(view: DeadLetterView, enabled: boolean) {
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<CauseGroupsState>({ page: DeadLetterGroups.empty, loading: true, error: null });
  const key = `${DeadLetterViewParams.key({ ...view, fingerprint: undefined })}#${nonce}`;
  const viewRef = useRef(view);
  useEffect(() => {
    viewRef.current = view;
  });
  useEffect(() => {
    if (!enabled) return;
    let live = true;
    const filter = { ...DeadLetterViewParams.filter(viewRef.current, services.clock.now()), fingerprint: undefined };
    actions.deadLetters.groups(filter).then(
      (page) => live && setState({ page, loading: false, error: null }),
      (error: unknown) => live && setState((s) => ({ ...s, loading: false, error: error instanceof Error ? error.message : String(error) })),
    );
    return () => {
      live = false;
    };
  }, [key, enabled]);
  const refresh = useCallback(() => {
    setState((s) => ({ ...s, loading: true }));
    setNonce((n) => n + 1);
  }, []);
  return { ...state, refresh };
}
