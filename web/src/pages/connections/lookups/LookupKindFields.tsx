import { Field, Select, TextArea, TextInput } from '../../../components/ui';
import {
  LibraryKind,
  LookupAuthMode,
  LookupAuthModeLabel,
  LookupSecretField,
  LookupSecretLabel,
  LookupSourceRules,
  LookupSslMode,
} from '../../../domain';
import { LibraryPicker } from '../../builder/studio/library/LibraryPicker';
import { SecretInput } from '../components/SecretInput';
import { ToggleField } from '../components/ToggleField';
import { LookupDraftPatch, LookupSecretView, type LookupFormStepProps } from './model/LookupSourceForm';
import { NumberText } from './model/NumberText';
import styles from './LookupSourceForm.module.css';

/** Texts of the kind settings. */
const KindText = {
  insecure: 'Skip TLS certificate verification',
  insecureHint: 'Only for test endpoints with self-signed certificates.',
  caPem: 'CA certificate (PEM, optional)',
  redisTls: 'Use TLS',
  readOnly: 'Every query runs in a READ ONLY transaction with this statement timeout; only parameterised SELECTs are accepted.',
  staticHint: 'Upload JSON or CSV in Library → Lookup tables; the source pins one version.',
  secretPlaceholder: 'Write-only, never shown again',
} as const;

/** Write-only secret input of one secret member (only rendered when the kind / auth mode uses it). */
function LookupSecret({ draft, stored, errors, update, field }: LookupFormStepProps & { field: LookupSecretField }) {
  if (!LookupSourceRules.secretFields(draft).includes(field)) return null;
  return (
    <Field label={LookupSecretLabel[field]}>
      <SecretInput value={LookupSecretView.of(draft, stored, field)} placeholder={KindText.secretPlaceholder} error={errors[`/secrets/${field}`]}
        onChange={(v) => update(LookupDraftPatch.secret(draft, field, v.value))} />
    </Field>
  );
}

/** HTTP: base URL, auth mode (+ username / header name and the matching secret), TLS. */
export function HttpFields(props: LookupFormStepProps) {
  const { draft, errors, update } = props;
  const h = draft.http;
  return (
    <>
      <Field label="Base URL" error={errors['/http/baseUrl']}>
        <TextInput controlSize="lg" mono sunken value={h.baseUrl} placeholder="https://crm.internal/api" onChange={(e) => update(LookupDraftPatch.http(draft, { baseUrl: e.target.value }))} />
      </Field>
      <Field label="Authentication">
        <Select controlSize="lg" sunken value={h.auth.mode} onChange={(e) => update(LookupDraftPatch.httpAuth(draft, { mode: e.target.value as LookupAuthMode }))}>
          {Object.values(LookupAuthMode).map((m) => <option key={m} value={m}>{LookupAuthModeLabel[m]}</option>)}
        </Select>
      </Field>
      {h.auth.mode === LookupAuthMode.Basic && (
        <Field label="Username" error={errors['/http/auth/username']}>
          <TextInput controlSize="lg" sunken value={h.auth.username} onChange={(e) => update(LookupDraftPatch.httpAuth(draft, { username: e.target.value }))} />
        </Field>
      )}
      {h.auth.mode === LookupAuthMode.Header && (
        <Field label="Header name" error={errors['/http/auth/headerName']}>
          <TextInput controlSize="lg" mono sunken value={h.auth.headerName} placeholder="X-Api-Key" onChange={(e) => update(LookupDraftPatch.httpAuth(draft, { headerName: e.target.value }))} />
        </Field>
      )}
      {Object.values(LookupSecretField).map((f) => <LookupSecret key={f} {...props} field={f} />)}
      <ToggleField label={KindText.insecure} hint={KindText.insecureHint} checked={h.tls.insecureSkipVerify} onChange={(v) => update(LookupDraftPatch.httpTls(draft, { insecureSkipVerify: v }))} />
      <Field label={KindText.caPem}>
        <TextArea controlSize="sm" mono rows={3} value={h.tls.caPem} onChange={(e) => update(LookupDraftPatch.httpTls(draft, { caPem: e.target.value }))} />
      </Field>
    </>
  );
}

/** Redis: addr, db, optional username + password, TLS. */
export function RedisFields(props: LookupFormStepProps) {
  const { draft, errors, update } = props;
  const r = draft.redis;
  return (
    <>
      <div className={styles.row}>
        <Field label="Address" error={errors['/redis/addr']}>
          <TextInput controlSize="lg" mono sunken value={r.addr} placeholder="redis:6379" onChange={(e) => update(LookupDraftPatch.redis(draft, { addr: e.target.value }))} />
        </Field>
        <Field label="Database" error={errors['/redis/db']}>
          <TextInput controlSize="lg" mono sunken type="number" value={NumberText.show(r.db)} onChange={(e) => update(LookupDraftPatch.redis(draft, { db: NumberText.of(e.target.value) }))} />
        </Field>
      </div>
      <Field label="Username (optional)">
        <TextInput controlSize="lg" sunken value={r.username} onChange={(e) => update(LookupDraftPatch.redis(draft, { username: e.target.value }))} />
      </Field>
      <LookupSecret {...props} field={LookupSecretField.Password} />
      <ToggleField label={KindText.redisTls} checked={r.tls} onChange={(v) => update(LookupDraftPatch.redis(draft, { tls: v }))} />
    </>
  );
}

/** Postgres (read-only): host, port, database, user, password, sslmode, statement timeout. */
export function PostgresFields(props: LookupFormStepProps) {
  const { draft, errors, update } = props;
  const p = draft.postgres;
  return (
    <>
      <div className={styles.row3}>
        <Field label="Host" error={errors['/postgres/host']}>
          <TextInput controlSize="lg" mono sunken value={p.host} onChange={(e) => update(LookupDraftPatch.postgres(draft, { host: e.target.value }))} />
        </Field>
        <Field label="Port" error={errors['/postgres/port']}>
          <TextInput controlSize="lg" mono sunken type="number" value={NumberText.show(p.port)} onChange={(e) => update(LookupDraftPatch.postgres(draft, { port: NumberText.of(e.target.value) }))} />
        </Field>
        <Field label="SSL mode">
          <Select controlSize="lg" sunken value={p.sslMode} onChange={(e) => update(LookupDraftPatch.postgres(draft, { sslMode: e.target.value as LookupSslMode }))}>
            {Object.values(LookupSslMode).map((m) => <option key={m} value={m}>{m}</option>)}
          </Select>
        </Field>
      </div>
      <div className={styles.row}>
        <Field label="Database" error={errors['/postgres/database']}>
          <TextInput controlSize="lg" mono sunken value={p.database} onChange={(e) => update(LookupDraftPatch.postgres(draft, { database: e.target.value }))} />
        </Field>
        <Field label="User" error={errors['/postgres/user']}>
          <TextInput controlSize="lg" mono sunken value={p.user} onChange={(e) => update(LookupDraftPatch.postgres(draft, { user: e.target.value }))} />
        </Field>
      </div>
      <LookupSecret {...props} field={LookupSecretField.Password} />
      <Field label="Statement timeout (ms)" hint={KindText.readOnly} error={errors['/postgres/statementTimeoutMs']}>
        <TextInput controlSize="lg" mono sunken type="number" value={NumberText.show(p.statementTimeoutMs)} onChange={(e) => update(LookupDraftPatch.postgres(draft, { statementTimeoutMs: NumberText.of(e.target.value) }))} />
      </Field>
    </>
  );
}

/** Static: a library lookup table pinned to a version. */
export function StaticFields({ draft, errors, update }: LookupFormStepProps) {
  return (
    <Field label="Library lookup table" hint={KindText.staticHint} error={errors['/static/tableRef'] ?? errors['/static/version']}>
      <LibraryPicker kind={LibraryKind.LookupTable} name={draft.static.tableRef} version={draft.static.version}
        onChange={(tableRef, version) => update(LookupDraftPatch.staticTable(draft, { tableRef, version }))} />
    </Field>
  );
}
