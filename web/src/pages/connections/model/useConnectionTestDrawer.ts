import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { QueryParam } from '../../../app/paths';
import type { Connection, ConnectionTestOptions } from '../../../domain';
import { actions, appStore } from '../../../store';

export type ConnectionTestDrawerState = {
  testId: string | null;
  openTest: (id: string) => void;
  showTest: (id: string) => void;
  closeTest: () => void;
  retest: (id: string, options?: ConnectionTestOptions) => void;
};

/**
 * Keeps the test drawer in the `?test=<id>` query param and starts a test when the drawer
 * opens for a connection that has no run in this session.
 */
export function useConnectionTestDrawer(connections: Connection[]): ConnectionTestDrawerState {
  const [params, setParams] = useSearchParams();
  const requested = params.get(QueryParam.Test);
  const known = !!requested && connections.some((c) => c.id === requested);
  const testId = known ? requested : null;

  useEffect(() => {
    if (testId && !appStore.getState().connectionTests[testId]) void actions.connections.test(testId);
  }, [testId]);

  const showTest = useCallback(
    (id: string | null) =>
      setParams((prev) => {
        const next = new URLSearchParams(prev);
        if (id) next.set(QueryParam.Test, id);
        else next.delete(QueryParam.Test);
        return next;
      }),
    [setParams],
  );

  const retest = useCallback((id: string, options?: ConnectionTestOptions) => void actions.connections.test(id, options), []);

  const openTest = useCallback(
    (id: string) => {
      if (id === testId) {
        retest(id);
        return;
      }
      actions.connections.clearTest(id);
      showTest(id);
    },
    [testId, retest, showTest],
  );

  return { testId, openTest, showTest, closeTest: () => showTest(null), retest };
}
