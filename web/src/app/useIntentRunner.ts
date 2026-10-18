import { useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { IntentType, type ActionIntent } from '../domain';
import { actions } from '../store';
import { AppPaths } from './paths';

/** Returns a function that performs an alert/attention intent (navigate or run an action). */
export function useIntentRunner(): (intent: ActionIntent) => void {
  const navigate = useNavigate();
  return useCallback(
    (intent: ActionIntent) => {
      switch (intent.type) {
        case IntentType.RouteDetail:
          return navigate(AppPaths.routeDetail(intent.routeId, intent.tab));
        case IntentType.RouteEdit:
          return navigate(AppPaths.routeEdit(intent.routeId, intent.step));
        case IntentType.TestConnection:
          return navigate(AppPaths.connections({ test: intent.connectionId }));
        case IntentType.TogglePause:
          return void actions.routes.togglePause(intent.routeId);
      }
    },
    [navigate],
  );
}
