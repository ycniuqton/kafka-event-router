import { useState } from 'react';
import { Button, ButtonVariant, Field, Modal, Select, TextInput } from '../../../components/ui';
import { RoleKeys, type NewUserDraft, type PasswordPolicy, type RoleKey, type Team } from '../../../domain';
import { actions } from '../../../store';
import { NewUserField, UserAdmin, UserAdminText, type FieldMessages } from '../model/UserAdmin';
import { GeneratedPasswordView, PasswordSourceField } from './PasswordFields';
import styles from './UsersSection.module.css';

export type CreateUserDialogProps = { teams: Team[]; policy: PasswordPolicy; onClose: () => void; onCreated: () => void };

/** Two steps: the form, then (when the server generated it) the one-time password. */
export function CreateUserDialog({ teams, policy, onClose, onCreated }: CreateUserDialogProps) {
  const [draft, setDraft] = useState<NewUserDraft>(UserAdmin.blankDraft());
  const [errors, setErrors] = useState<FieldMessages>({});
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generated, setGenerated] = useState<string | null>(null);
  const submit = async () => {
    const local = UserAdmin.draftErrors(draft, policy);
    setErrors(local);
    if (Object.keys(local).length > 0) return;
    setBusy(true);
    try {
      const created = await UserAdmin.create(draft);
      actions.toast.show(UserAdminText.Created(created.members[0]?.email ?? draft.email));
      onCreated();
      if (created.generatedPassword) setGenerated(created.generatedPassword);
      else onClose();
    } catch (error) {
      const failure = UserAdmin.failure(error);
      setErrors(failure.fields);
      setMessage(failure.message);
    } finally {
      setBusy(false);
    }
  };
  const footer = generated ? (
    <div className={styles.footer}><Button onClick={onClose}>{UserAdminText.Done}</Button></div>
  ) : (
    <div className={styles.footer}>
      <Button variant={ButtonVariant.Secondary} onClick={onClose}>{UserAdminText.Cancel}</Button>
      <Button variant={ButtonVariant.Primary} disabled={busy} onClick={() => void submit()}>{UserAdminText.CreateTitle}</Button>
    </div>
  );
  return (
    <Modal open onClose={onClose} title={UserAdminText.CreateTitle} subtitle={UserAdminText.CreateSubtitle} footer={footer}>
      {generated ? <GeneratedPasswordView password={generated} /> : (
        <NewUserForm draft={draft} teams={teams} policy={policy} errors={errors} message={message} onChange={setDraft} />
      )}
    </Modal>
  );
}

type NewUserFormProps = { draft: NewUserDraft; teams: Team[]; policy: PasswordPolicy; errors: FieldMessages; message: string | null; onChange: (d: NewUserDraft) => void };

/** Email, display name, password source, workspace admin and an optional first team role. */
function NewUserForm({ draft, teams, policy, errors, message, onChange }: NewUserFormProps) {
  return (
    <div className={styles.form}>
      {message && <div className={styles.error} role="alert">{message}</div>}
      <Field label={UserAdminText.Email} error={errors[NewUserField.Email]}>
        <TextInput block autoFocus type="email" value={draft.email} aria-label={UserAdminText.Email} onChange={(e) => onChange({ ...draft, email: e.target.value })} />
      </Field>
      <Field label={UserAdminText.DisplayName} error={errors[NewUserField.DisplayName]}>
        <TextInput block value={draft.displayName} aria-label={UserAdminText.DisplayName} onChange={(e) => onChange({ ...draft, displayName: e.target.value })} />
      </Field>
      <PasswordSourceField source={draft.passwordSource} password={draft.password} policy={policy} error={errors[NewUserField.Password]}
        onChange={(passwordSource, password) => onChange({ ...draft, passwordSource, password })} />
      <label className={styles.check}>
        <input type="checkbox" checked={draft.workspaceAdmin} onChange={(e) => onChange({ ...draft, workspaceAdmin: e.target.checked })} />
        {UserAdminText.WorkspaceAdmin}
      </label>
      <TeamRoleFields draft={draft} teams={teams} errors={errors} onChange={onChange} />
    </div>
  );
}

type TeamRoleFieldsProps = { draft: NewUserDraft; teams: Team[]; errors: FieldMessages; onChange: (d: NewUserDraft) => void };

/** Optional first team; the role applies only when a team is picked. */
function TeamRoleFields({ draft, teams, errors, onChange }: TeamRoleFieldsProps) {
  return (
    <>
      <Field label={UserAdminText.Team} error={errors[NewUserField.TeamId]}>
        <Select block value={draft.teamId ?? ''} aria-label={UserAdminText.Team} onChange={(e) => onChange({ ...draft, teamId: e.target.value || null })}>
          <option value="">{UserAdminText.NoTeam}</option>
          {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </Select>
      </Field>
      <Field label={UserAdminText.Role} error={errors[NewUserField.Role]}>
        <Select block value={draft.role} disabled={!draft.teamId} aria-label={UserAdminText.Role} onChange={(e) => onChange({ ...draft, role: e.target.value as RoleKey })}>
          {RoleKeys.assignable.map((r) => <option key={r} value={r}>{RoleKeys.toRole(r)}</option>)}
        </Select>
      </Field>
    </>
  );
}
