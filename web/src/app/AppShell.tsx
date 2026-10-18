import { Suspense, useEffect } from 'react';
import { Outlet, useMatches } from 'react-router-dom';
import { ToastHost } from '../components/ui';
import { actions, Keys, useShortcut } from '../store';
import styles from './AppShell.module.css';
import type { RouteHandle } from './navigation';
import { NewRouteModal } from './NewRouteModal';
import { SideNav } from './SideNav';
import { TopBar } from './TopBar';

/** Reads the deepest route `handle` (nav key, fullscreen flag). */
function useRouteHandle(): RouteHandle {
  const matches = useMatches();
  const handles = matches.map((m) => m.handle as RouteHandle | undefined).filter(Boolean) as RouteHandle[];
  return handles[handles.length - 1] ?? {};
}

/** App frame: top bar, left nav (hidden in the builder), routed page, chooser modal and toast. */
export function AppShell() {
  const handle = useRouteHandle();
  useEffect(() => {
    void actions.bootstrap();
  }, []);
  useShortcut({ key: Keys.N, handler: () => actions.openNewRouteChooser() });
  return (
    <div className={styles.shell}>
      <TopBar />
      <div className={styles.body}>
        {!handle.fullscreen && <SideNav active={handle.nav} />}
        <main className={styles.main}>
          <Suspense fallback={<div className={styles.loading}>Loading…</div>}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <NewRouteModal />
      <ToastHost />
    </div>
  );
}
