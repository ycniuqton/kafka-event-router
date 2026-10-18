import { useState } from 'react';
import { Button, ButtonVariant, Field, Modal, Select, TextInput } from '../../../components/ui';
import { RoleKeys, type InviteDraft, type RoleKey, type Team } from '../../../domain';
import { actions } from '../../../store';
import { MemberRoles } from '../model/MemberRoles';

export type InviteDialogProps = { teams: Team[]; open: boolean; onClose: () => void; onInvited: () => void };

/** Pre-invite someone by email with a team role; the row is claimed at their first sign-in. */
export function InviteDialog({ teams, open, onClose, onInvited }: InviteDialogProps) {
  const [draft, setDraft] = useState<InviteDraft>(MemberRoles.blankInvite(teams));
  const [busy, setBusy] = useState(false);
  const send = async () => {
    setBusy(true);
    try {
      actions.toast.show(await MemberRoles.invite(draft));
      onInvited();
      onClose();
    } catch (error) {
      actions.toast.show((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <Button variant={ButtonVariant.Primary} disabled={busy || !draft.email.trim() || !draft.teamId} onClick={() => void send()}>
      Invite
    </Button>
  );
  return (
    <Modal open={open} onClose={onClose} title="Invite a member" subtitle="They get this role when they first sign in." footer={footer}>
      <Field label="Email">
        <TextInput block type="email" value={draft.email} aria-label="Email" onChange={(e) => setDraft({ ...draft, email: e.target.value })} />
      </Field>
      <Field label="Display name" hint="Optional — defaults to the part before @.">
        <TextInput block value={draft.displayName} aria-label="Display name" onChange={(e) => setDraft({ ...draft, displayName: e.target.value })} />
      </Field>
      <Field label="Team">
        <Select block value={draft.teamId} aria-label="Team" onChange={(e) => setDraft({ ...draft, teamId: e.target.value })}>
          {teams.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Role">
        <Select block value={draft.role} aria-label="Role" onChange={(e) => setDraft({ ...draft, role: e.target.value as RoleKey })}>
          {RoleKeys.assignable.map((r) => (
            <option key={r} value={r}>
              {RoleKeys.toRole(r)}
            </option>
          ))}
        </Select>
      </Field>
    </Modal>
  );
}
