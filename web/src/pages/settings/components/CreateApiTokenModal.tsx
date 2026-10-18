import { useState } from 'react';
import { services } from '../../../api';
import { Button, Field, Modal, SegmentedControl, Select, TextInput } from '../../../components/ui';
import { ApiTokenRules, type ApiTokenDraft, type CreatedApiToken } from '../../../domain';
import { actions } from '../../../store';
import { ApiTokenRows, ApiTokenText } from '../model/ApiTokenRows';
import styles from '../ApiTokensPanel.module.css';

type Props = { open: boolean; onClose: () => void; onCreated: () => void };

/** Two steps: the form, then the one-time secret with a copy button. */
export function CreateApiTokenModal({ open, onClose, onCreated }: Props) {
  const [draft, setDraft] = useState<ApiTokenDraft>(ApiTokenRules.emptyDraft());
  const [created, setCreated] = useState<CreatedApiToken | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const close = () => {
    setDraft(ApiTokenRules.emptyDraft());
    setCreated(null);
    setError(null);
    onClose();
  };
  const submit = async () => {
    const local = ApiTokenRules.nameProblem(draft);
    if (local) return setError(local);
    setBusy(true);
    try {
      setCreated(await services.apiTokens.create(draft));
      setError(null);
      onCreated();
    } catch (e) {
      setError(ApiTokenRows.createError(e, draft));
    } finally {
      setBusy(false);
    }
  };
  const footer = created ? (
    <div className={styles.footer}><Button onClick={close}>{ApiTokenText.done}</Button></div>
  ) : (
    <div className={styles.footer}>
      <Button variant="secondary" onClick={close}>{ApiTokenText.cancel}</Button>
      <Button disabled={busy} onClick={submit}>{busy ? ApiTokenText.creating : ApiTokenText.create}</Button>
    </div>
  );
  return (
    <Modal open={open} onClose={close} title={ApiTokenText.modalTitle} subtitle={ApiTokenText.modalSubtitle} footer={footer}>
      {created ? <SecretView secret={created.secret} /> : <TokenForm draft={draft} error={error} onChange={setDraft} />}
    </Modal>
  );
}

/** Name, scope and expiry inputs. */
function TokenForm({ draft, error, onChange }: { draft: ApiTokenDraft; error: string | null; onChange: (d: ApiTokenDraft) => void }) {
  return (
    <div className={styles.form}>
      <Field label={ApiTokenText.name} error={error ?? undefined}>
        <TextInput block autoFocus value={draft.name} maxLength={ApiTokenRules.maxNameLength} placeholder={ApiTokenText.namePlaceholder}
          onChange={(e) => onChange({ ...draft, name: e.target.value })} />
      </Field>
      <Field label={ApiTokenText.scope} hint={ApiTokenRules.scopeHelp[draft.scope]}>
        <SegmentedControl options={ApiTokenRules.scopes} value={draft.scope} onChange={(scope) => onChange({ ...draft, scope })} />
      </Field>
      <Field label={ApiTokenText.expires}>
        <Select value={String(draft.expiresInDays)} onChange={(e) => onChange({ ...draft, expiresInDays: Number(e.target.value) })}>
          {ApiTokenRows.expiryOptions().map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </Select>
      </Field>
    </div>
  );
}

/** The one-time secret. */
function SecretView({ secret }: { secret: string }) {
  const copy = async () => {
    await navigator.clipboard?.writeText(secret);
    actions.toast.show(ApiTokenText.copied);
  };
  return (
    <div className={styles.form}>
      <strong>{ApiTokenText.secretTitle}</strong>
      <div className={styles.secretBox}>
        <span className={styles.secretValue} data-testid="api-token-secret">{secret}</span>
        <Button size="sm" variant="secondary" onClick={copy}>{ApiTokenText.copy}</Button>
      </div>
      <p className={styles.secretNote}>{ApiTokenText.secretBody}</p>
    </div>
  );
}
