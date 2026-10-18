import { useEffect, useMemo, useSyncExternalStore } from 'react';
import { services } from '../../api';
import { PreviewActions } from './PreviewActions';
import type { PreviewState } from './PreviewState';
import { PreviewStore } from './PreviewStore';

/** One sample panel's state and actions; the session is closed when the component unmounts. */
export function usePreviewSession(): { state: PreviewState; actions: PreviewActions } {
  const actions = useMemo(() => new PreviewActions(new PreviewStore(), services.preview), []);
  const state = useSyncExternalStore(actions.store.subscribe, actions.store.getState);
  useEffect(() => () => actions.dispose(), [actions]);
  return { state, actions };
}
