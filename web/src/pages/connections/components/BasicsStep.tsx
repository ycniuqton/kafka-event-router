import { Field, SegmentedControl, TextInput } from '../../../components/ui';
import { EnvMeta, EnvTag } from '../../../domain';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import { ConnectionFormModel } from '../model/ConnectionForm';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { GrantedTeams } from './GrantedTeams';
import { ToggleField } from './ToggleField';
import styles from './BasicsStep.module.css';

/** Environments in the order the design lists them. */
const EnvOrder = [EnvTag.Dev, EnvTag.Staging, EnvTag.Prod] as const;

/** Texts of the Basics step. */
const BasicsText = {
  nameLocked: 'The name is the id routes refer to, so it cannot change.',
  ownerNew: 'New connections belong to your team.',
  ownerEdit: 'The owner team edits and tests this connection; other teams use it through grants.',
  topicCreate: 'Allow creating topics',
  topicCreateHint: 'Lets editors create topics on this cluster from the topic browser and the route builder.',
  probe: 'Health check every 60 s',
  probeHint: 'Re-tests brokers, authentication and topic listing in the background and updates the status.',
  sensitive: 'Sensitive data',
  sensitiveHint: 'Masks event values in samples, peeks, dead letters and live tail for every role (viewers are always masked).',
} as const;

/** Step 1 — name, description, environment tag, owner team and the per-connection opt-ins. */
export function BasicsStep({ form, set, editing, errors, connectionId }: ConnectionFormStepProps) {
  const envOptions = EnvOrder.map((env) => ({
    value: env,
    label: (
      <span className={styles.env} data-tone={EnvMeta[env].tone}>
        {EnvMeta[env].label}
      </span>
    ),
  }));
  const error = (field: string) => ConnectionFieldErrors.of(errors, field);
  return (
    <>
      <Field label="Name" hint={editing ? BasicsText.nameLocked : undefined} error={error(ConnectionField.Name)}>
        <TextInput
          controlSize="lg"
          mono
          sunken
          value={form.name}
          disabled={editing}
          placeholder="orders-prod"
          onChange={(e) => set('name', ConnectionFormModel.normalizeName(e.target.value))}
        />
      </Field>
      <Field label="Description" error={error(ConnectionField.Description)}>
        <TextInput controlSize="lg" sunken value={form.description} onChange={(e) => set('description', e.target.value)} />
      </Field>
      <div className={styles.group}>
        <span className={styles.label}>Environment</span>
        <SegmentedControl size="md" options={envOptions} value={form.env} onChange={(env) => set('env', env)} ariaLabel="Environment" />
        {error(ConnectionField.Env) && <span className={styles.error}>{error(ConnectionField.Env)}</span>}
      </div>
      <Field label="Owner team" hint={editing ? BasicsText.ownerEdit : BasicsText.ownerNew} error={error(ConnectionField.Team)}>
        <TextInput controlSize="lg" sunken value={form.team} disabled readOnly />
      </Field>
      {editing && connectionId && <GrantedTeams connectionId={connectionId} />}
      <ToggleField label={BasicsText.sensitive} hint={BasicsText.sensitiveHint} checked={form.sensitive} onChange={(v) => set('sensitive', v)} />
      <ToggleField label={BasicsText.topicCreate} hint={BasicsText.topicCreateHint} checked={form.allowTopicCreate} onChange={(v) => set('allowTopicCreate', v)} />
      <ToggleField label={BasicsText.probe} hint={BasicsText.probeHint} checked={form.probeEnabled} onChange={(v) => set('probeEnabled', v)} />
    </>
  );
}
