import { Field, SegmentedControl, TextInput } from '../../../components/ui';
import { EnvMeta, EnvTag } from '../../../domain';
import { ToggleField } from '../components/ToggleField';
import { LookupDraftPatch, type LookupFormStepProps } from './model/LookupSourceForm';
import styles from './LookupSourceForm.module.css';

/** Environments in the order the connections form lists them. */
const EnvOrder = [EnvTag.Dev, EnvTag.Staging, EnvTag.Prod] as const;

/** Texts of the Basics step. */
const LookupBasicsText = {
  nameLocked: 'Routes refer to the source by id; the name is shown in the Studio.',
  ownerNew: 'New lookup sources belong to your team.',
  ownerEdit: 'The owner team edits and tests this source.',
  yourTeam: 'Your team',
  sensitive: 'Sensitive data',
  sensitiveHint: 'Masks event values in previews that use this source, like a sensitive connection.',
} as const;

/** Step 1 — name, description, environment, owner team and the sensitive flag. */
export function LookupBasicsStep({ draft, editing, teamName, errors, update }: LookupFormStepProps) {
  const envOptions = EnvOrder.map((env) => ({ value: env, label: EnvMeta[env].label }));
  return (
    <>
      <Field label="Name" hint={editing ? LookupBasicsText.nameLocked : undefined} error={errors['/name']}>
        <TextInput controlSize="lg" mono sunken value={draft.name} placeholder="crm-api" onChange={(e) => update(LookupDraftPatch.set(draft, 'name', e.target.value))} />
      </Field>
      <Field label="Description" error={errors['/description']}>
        <TextInput controlSize="lg" sunken value={draft.description} onChange={(e) => update(LookupDraftPatch.set(draft, 'description', e.target.value))} />
      </Field>
      <div className={styles.group}>
        <span className={styles.label}>Environment</span>
        <SegmentedControl size="md" options={envOptions} value={draft.envTag} onChange={(env) => update(LookupDraftPatch.set(draft, 'envTag', env))} ariaLabel="Environment" />
      </div>
      <Field label="Owner team" hint={editing ? LookupBasicsText.ownerEdit : LookupBasicsText.ownerNew}>
        <TextInput controlSize="lg" sunken value={teamName || LookupBasicsText.yourTeam} disabled readOnly />
      </Field>
      <ToggleField label={LookupBasicsText.sensitive} hint={LookupBasicsText.sensitiveHint} checked={draft.sensitive} onChange={(v) => update(LookupDraftPatch.set(draft, 'sensitive', v))} />
    </>
  );
}
