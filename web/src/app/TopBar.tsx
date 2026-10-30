import { useRef, type ChangeEvent } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Kbd } from '../components/ui';
import { ApprovalsMenu, Can } from '../access';
import { Permission, Theme } from '../domain';
import { actions, Keys, settingsStore, useCurrentUser, useSettings, useShortcut } from '../store';
import { AlertsBell } from './AlertsBell';
import { AppPaths, QueryParam } from './paths';
import { UserMenu } from './UserMenu';
import styles from './TopBar.module.css';

/** 52px top bar: logo, global search ("/"), alerts, theme toggle, account menu, "+ New route". */
export function TopBar() {
  const { theme } = useSettings();
  const user = useCurrentUser();
  return (
    <header className={styles.bar}>
      <Link to={AppPaths.dashboard()} className={styles.brand}>
        <span className={styles.logo}>
          <span className={styles.logoDot} />
        </span>
        <span className={styles.name}>Event Router</span>
      </Link>
      <GlobalSearch />
      <div className={styles.spacer} />
      <ApprovalsMenu />
      <AlertsBell />
      <Button size="lg" className={styles.themeButton} onClick={() => settingsStore.toggleTheme()}>
        {theme === Theme.Dark ? 'Light theme' : 'Dark theme'}
      </Button>
      {user && <UserMenu user={user} />}
      <Can permission={Permission.RouteWrite}>
        <Button variant="primary" size="lg" onClick={() => actions.openNewRouteChooser()}>
          + New route
        </Button>
      </Can>
    </header>
  );
}

/** Search box bound to `/routes?q=` (the URL is the single source of truth); "/" focuses it. */
function GlobalSearch() {
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const location = useLocation();
  const [params] = useSearchParams();
  const onRoutes = location.pathname === AppPaths.routes();
  const text = onRoutes ? (params.get(QueryParam.Search) ?? '') : '';
  useShortcut({ key: Keys.Slash, handler: () => inputRef.current?.focus() });
  const onChange = (e: ChangeEvent<HTMLInputElement>) => navigate(AppPaths.routes({ q: e.target.value }), { replace: onRoutes });
  return (
    <div className={styles.search}>
      <input
        ref={inputRef}
        value={text}
        onChange={onChange}
        placeholder="Search routes, topics, connections"
        className={styles.searchInput}
        aria-label="Search"
      />
      <span className={styles.searchHint}>
        <Kbd>/</Kbd>
      </span>
    </div>
  );
}
