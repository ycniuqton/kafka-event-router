import type { SessionUser } from '../../api/http/core';
import { Button, ButtonVariant } from '../../components/ui';
import { PasswordMode } from './AuthConfigStore';
import { useAuthConfig } from './useAuthConfig';
import { ChangePasswordForm } from './ChangePasswordForm';
import { SessionServices } from './SessionServices';
import styles from './Auth.module.css';

/** Forced change screen copy. */
const ForcedChangeText = {
  Title: 'Change your password',
  Subtitle: 'Your password was set by an administrator. Choose a new one before you continue.',
  SignOut: 'Sign out',
} as const;

export type PasswordChangeScreenProps = { user: SessionUser; onChanged: (user: SessionUser) => void };

/** Shown instead of the app while `must_change_password` is true: no shell, no data calls, only the change and sign out. */
export function PasswordChangeScreen({ user, onChanged }: PasswordChangeScreenProps) {
  const policy = PasswordMode.policy(useAuthConfig());
  const signOut = (
    <Button type="button" variant={ButtonVariant.Ghost} onClick={() => void SessionServices.signOut()}>
      {ForcedChangeText.SignOut}
    </Button>
  );
  return (
    <div className={styles.screen}>
      <div className={styles.card}>
        <div>
          <h1 className={styles.title}>{ForcedChangeText.Title}</h1>
          <p className={styles.subtitle}>{ForcedChangeText.Subtitle}</p>
        </div>
        <p className={styles.muted}>{user.email}</p>
        <ChangePasswordForm policy={policy} onChanged={onChanged} actions={signOut} />
      </div>
    </div>
  );
}
