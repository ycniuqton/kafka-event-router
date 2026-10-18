import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { Button, ButtonSize } from '../../components/ui';
import { actions } from '../../store';
import { AuthPaths, CurrentLocation } from './AuthPaths';
import { GlobalApiErrorHandlers } from './GlobalApiErrorHandlers';
import { GateStatus, SessionGateController, type GateOutcome } from './SessionGateController';
import { SessionServices } from './SessionServices';
import styles from './Auth.module.css';

/** Gate copy. */
const GateText = { Checking: 'Loading…', Retry: 'Retry' } as const;

/** Resolves the session once per attempt and installs the global API error reactions. */
function useSessionGate(): { outcome: GateOutcome; retry: () => void } {
  const navigate = useNavigate();
  const controller = useMemo(() => new SessionGateController(SessionServices.current()), []);
  const [outcome, setOutcome] = useState<GateOutcome>({ status: GateStatus.Checking });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    void controller.resolve(CurrentLocation.path()).then((next) => alive && setOutcome(next));
    return () => {
      alive = false;
    };
  }, [controller, attempt]);

  useEffect(
    () =>
      GlobalApiErrorHandlers.install(SessionServices.httpContext().errors, {
        toLogin: () => navigate(AuthPaths.loginUrl(CurrentLocation.path()), { replace: true }),
        toast: (message) => actions.toast.show(message),
      }),
    [navigate],
  );

  return { outcome, retry: () => setAttempt((n) => n + 1) };
}

/** Renders the app only for a signed-in user; otherwise redirects to `/login?return_to=<here>`. */
export function SessionGate({ children }: { children: ReactNode }) {
  const { outcome, retry } = useSessionGate();
  if (outcome.status === GateStatus.Authenticated) return <>{children}</>;
  if (outcome.status === GateStatus.Redirect) return <Navigate to={outcome.to} replace />;
  if (outcome.status === GateStatus.Failed) {
    return (
      <div className={styles.gate}>
        <span>{outcome.message}</span>
        <Button size={ButtonSize.Sm} onClick={retry}>
          {GateText.Retry}
        </Button>
      </div>
    );
  }
  return <div className={styles.gate}>{GateText.Checking}</div>;
}
