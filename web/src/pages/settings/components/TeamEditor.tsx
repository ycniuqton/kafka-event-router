import { useState } from 'react';
import { Button, ButtonVariant, Field, Modal, SegmentedControl, TextInput } from '../../../components/ui';
import type { Team, TeamDraft } from '../../../domain';
import { actions } from '../../../store';
import { TeamAdmin } from '../model/TeamAdmin';

/** Values of the approval-policy switch. */
const PolicyChoice = { Required: 'required', Direct: 'direct' } as const;
type PolicyChoice = (typeof PolicyChoice)[keyof typeof PolicyChoice];

const PolicyOptions = [
  { value: PolicyChoice.Required, label: 'Require approval' },
  { value: PolicyChoice.Direct, label: 'Approvers deploy directly' },
];

export type TeamEditorProps = { team: Team | null; open: boolean; onClose: () => void; onSaved: () => void };

/** Create or edit a team: name and whether production deploys need a second person (strict 4-eyes). */
export function TeamEditor({ team, open, onClose, onSaved }: TeamEditorProps) {
  const [draft, setDraft] = useState<TeamDraft>(team ? TeamAdmin.draftOf(team) : TeamAdmin.blankDraft());
  const [busy, setBusy] = useState(false);
  const run = async (work: () => Promise<string>) => {
    setBusy(true);
    try {
      actions.toast.show(await work());
      onSaved();
      onClose();
    } catch (error) {
      actions.toast.show((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <>
      {team && (
        <Button variant={ButtonVariant.Danger} disabled={busy} onClick={() => void run(() => TeamAdmin.remove(team))}>
          Delete team
        </Button>
      )}
      <Button variant={ButtonVariant.Primary} disabled={busy || !TeamAdmin.canSave(draft)} onClick={() => void run(() => TeamAdmin.save(team, draft))}>
        {team ? 'Save team' : 'Create team'}
      </Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title={team ? `Edit ${team.name}` : 'New team'} subtitle="Routes and connections belong to a team." footer={footer}>
      <Field label="Name">
        <TextInput block value={draft.name} maxLength={100} aria-label="Team name" onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
      </Field>
      <Field label="Production deploys" hint="With approval required, every deploy to a prod connection needs someone else to approve.">
        <SegmentedControl
          options={PolicyOptions}
          value={draft.requireProdApproval ? PolicyChoice.Required : PolicyChoice.Direct}
          onChange={(v: PolicyChoice) => setDraft({ ...draft, requireProdApproval: v === PolicyChoice.Required })}
          ariaLabel="Production deploys"
        />
      </Field>
    </Modal>
  );
}
