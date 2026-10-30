import { useEffect, useRef, useState } from 'react';
import type { CurrentUser } from '../domain';
import { Keys, useShortcut } from '../store';
import { ChangePasswordModal, PasswordMode, SessionServices, useAuthConfig } from './auth';
import styles from './UserMenu.module.css';

/** Copy of the user menu. */
const UserMenuText = { Open: 'Account menu', ChangePassword: 'Change password', SignOut: 'Sign out' } as const;

/** Avatar button with the account menu: change password (password mode only) and sign out. No sign-up: admins create users. */
export function UserMenu({ user }: { user: CurrentUser }) {
  const passwordMode = PasswordMode.enabled(useAuthConfig());
  const [open, setOpen] = useState(false);
  const [changing, setChanging] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: () => setOpen(false) }, open);
  useEffect(() => {
    if (!open) return undefined;
    const onDown = (e: MouseEvent) => !wrapRef.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [open]);
  const startChange = () => {
    setOpen(false);
    setChanging(true);
  };
  return (
    <div className={styles.wrap} ref={wrapRef}>
      <button type="button" className={styles.avatar} title={`${user.name} · ${user.role}`} aria-label={UserMenuText.Open} aria-haspopup="menu" aria-expanded={open}
        onClick={() => setOpen(!open)}>
        {user.initials}
      </button>
      {open && (
        <div className={styles.menu} role="menu">
          <div className={styles.who}>
            <span className={styles.name}>{user.name}</span>
            <span className={styles.meta}>{`${user.role} · ${user.workspace}`}</span>
          </div>
          {passwordMode && (
            <button type="button" role="menuitem" className={styles.item} onClick={startChange}>
              {UserMenuText.ChangePassword}
            </button>
          )}
          <button type="button" role="menuitem" className={styles.item} onClick={() => void SessionServices.signOut()}>
            {UserMenuText.SignOut}
          </button>
        </div>
      )}
      <ChangePasswordModal open={changing} onClose={() => setChanging(false)} />
    </div>
  );
}
