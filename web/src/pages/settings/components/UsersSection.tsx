import { useState } from 'react';
import { services } from '../../../api';
import { accessStore, useCan } from '../../../access';
import { PasswordMode, useAuthConfig } from '../../../app/auth';
import { Button, ButtonSize, ButtonVariant, ToneBadge } from '../../../components/ui';
import { Permission, type PasswordPolicy, type Team, type TeamMember } from '../../../domain';
import { actions, useServiceQuery } from '../../../store';
import { UserAdmin, UserAdminText } from '../model/UserAdmin';
import { CreateUserDialog } from './CreateUserDialog';
import { DisableUserDialog } from './DisableUserDialog';
import { ResetPasswordDialog } from './ResetPasswordDialog';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import sectionStyles from './SettingsSection.module.css';
import styles from './UsersSection.module.css';

const UserColumns = '1fr 1fr 80px 210px';

/** The open user dialog, if any. */
const UserDialog = { Create: 'create', Reset: 'reset', Disable: 'disable' } as const;
type UserDialog = (typeof UserDialog)[keyof typeof UserDialog];
type OpenDialog = { kind: UserDialog; user: TeamMember | null } | null;

/** Workspace admins (`user.manage`): one row per user; create password users, reset passwords, disable / enable. */
export function UsersSection({ members, onChanged }: { members: TeamMember[]; onChanged: () => void }) {
  const canManage = useCan(Permission.UserManage);
  const config = useAuthConfig();
  const passwordMode = PasswordMode.enabled(config);
  const policy = PasswordMode.policy(config);
  const { data: teams } = useServiceQuery(() => services.teams.list(), []);
  const [dialog, setDialog] = useState<OpenDialog>(null);
  if (!canManage) return null;
  const close = () => setDialog(null);
  const changed = () => {
    void accessStore.refresh();
    onChanged();
  };
  const hint = (
    <>
      {passwordMode ? UserAdminText.PasswordHint : UserAdminText.Hint}{' '}
      {passwordMode && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setDialog({ kind: UserDialog.Create, user: null })}>
          {UserAdminText.Create}
        </Button>
      )}
    </>
  );
  return (
    <SettingsSection title={UserAdminText.Title} hint={hint}>
      {UserAdmin.users(members).map((u) => (
        <UserRow key={u.userId} user={u} passwordMode={passwordMode} onOpen={(kind) => setDialog({ kind, user: u })} onChanged={changed} />
      ))}
      <UserDialogs dialog={dialog} teams={teams ?? []} policy={policy} onClose={close} onChanged={changed} />
    </SettingsSection>
  );
}

type UserDialogsProps = { dialog: OpenDialog; teams: Team[]; policy: PasswordPolicy; onClose: () => void; onChanged: () => void };

/** Mounts the open dialog (fresh state each time it opens). */
function UserDialogs({ dialog, teams, policy, onClose, onChanged }: UserDialogsProps) {
  if (dialog?.kind === UserDialog.Create) return <CreateUserDialog teams={teams} policy={policy} onClose={onClose} onCreated={onChanged} />;
  if (dialog?.kind === UserDialog.Reset && dialog.user) return <ResetPasswordDialog user={dialog.user} policy={policy} onClose={onClose} onReset={onChanged} />;
  if (dialog?.kind === UserDialog.Disable && dialog.user) return <DisableUserDialog user={dialog.user} onClose={onClose} onDone={onChanged} />;
  return null;
}

type UserRowProps = { user: TeamMember; passwordMode: boolean; onOpen: (kind: UserDialog) => void; onChanged: () => void };

/** Name with badges, email, sign-in method and the row actions. */
function UserRow({ user, passwordMode, onOpen, onChanged }: UserRowProps) {
  const disabled = UserAdmin.isDisabled(user);
  const enable = async () => {
    try {
      actions.toast.show(await UserAdmin.setDisabled(user, false));
      onChanged();
    } catch (error) {
      actions.toast.show(UserAdmin.failure(error).message ?? String(error));
    }
  };
  return (
    <SettingsGridRow columns={UserColumns}>
      <span className={styles.name}>
        {user.name}
        {UserAdmin.badges(user).map((b) => <ToneBadge key={b.label} tone={b.tone}>{b.label}</ToneBadge>)}
      </span>
      <span className={sectionStyles.mono}>{user.email}</span>
      <span className={sectionStyles.muted}>{UserAdmin.signIn(user)}</span>
      <span className={sectionStyles.actions}>
        {passwordMode && !disabled && (
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => onOpen(UserDialog.Reset)}>{UserAdminText.Reset}</Button>
        )}
        {disabled ? (
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void enable()}>{UserAdminText.Enable}</Button>
        ) : (
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => onOpen(UserDialog.Disable)}>{UserAdminText.Disable}</Button>
        )}
      </span>
    </SettingsGridRow>
  );
}
