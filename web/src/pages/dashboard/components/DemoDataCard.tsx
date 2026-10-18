import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Button, Mono } from '../../../components/ui';
import { DemoDataText, type OnboardingState } from '../../../domain';
import { DemoDataActions } from '../model/DemoData';
import styles from './FirstRunChecklist.module.css';

/** Demo-data shortcut of the checklist: create the in-memory demo connection, open the builder, or remove it. */
export function DemoDataCard() {
  const navigate = useNavigate();
  const [state, setState] = useState<OnboardingState | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let live = true;
    void DemoDataActions.load()
      .then((s) => live && setState(s))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);
  const run = (step: () => Promise<OnboardingState>) => {
    setBusy(true);
    void step()
      .then(setState)
      .finally(() => setBusy(false));
  };
  if (state && !state.demoAvailable) return null;
  if (state?.demo) {
    return (
      <div className={styles.demo} data-testid="demo-data">
        <span>
          Demo data is live: <Mono>{state.demo.topic}</Mono> receives sample orders on the in-memory demo connection.
        </span>
        <span className={styles.demoActions}>
          <Button size="sm" variant="primary" onClick={() => navigate(AppPaths.routeNew())}>Build a route</Button>
          <Button size="sm" variant="ghost" onClick={() => run(DemoDataActions.remove)} disabled={busy}>Remove demo data</Button>
        </span>
      </div>
    );
  }
  return (
    <div className={styles.demo} data-testid="demo-data">
      <span>
        Just exploring? Try the builder with a demo connection and a live <Mono>{DemoDataText.topic}</Mono> topic.
      </span>
      <Button size="sm" onClick={() => run(DemoDataActions.create)} disabled={busy || !state}>
        Try with demo data
      </Button>
    </div>
  );
}
