import { Link } from 'react-router-dom';
import { Can } from '../access';
import { useAppSelector, useCurrentUser, useSettings } from '../store';
import { NavItems, type NavItem, type NavKey } from './navigation';
import styles from './SideNav.module.css';

/** Left navigation: 7 entries with live counts and optional phase tags, plus workspace footer. */
export function SideNav({ active }: { active?: NavKey }) {
  const { showPhases } = useSettings();
  const user = useCurrentUser();
  return (
    <nav className={styles.nav} aria-label="Main">
      {NavItems.map((item) =>
        item.permission ? (
          <Can key={item.key} permission={item.permission}>
            <NavEntry item={item} active={item.key === active} showPhase={showPhases} />
          </Can>
        ) : (
          <NavEntry key={item.key} item={item} active={item.key === active} showPhase={showPhases} />
        ),
      )}
      <div className={styles.spacer} />
      <div className={styles.footer}>
        <div className={styles.workspace}>{user?.workspace}</div>
        <div>{user && `${user.name} · ${user.role}`}</div>
        <div className={styles.hints}>N new · / search · ⌘S save</div>
      </div>
    </nav>
  );
}

function NavEntry({ item, active, showPhase }: { item: NavItem; active: boolean; showPhase: boolean }) {
  const count = useAppSelector((s) => item.count?.(s));
  return (
    <Link to={item.path} className={[styles.item, active && styles.active].filter(Boolean).join(' ')} aria-current={active ? 'page' : undefined}>
      <span className={styles.label}>{item.label}</span>
      {count !== undefined && <span className={styles.count}>{count}</span>}
      {showPhase && <span className={styles.phase}>{item.phase}</span>}
    </Link>
  );
}
