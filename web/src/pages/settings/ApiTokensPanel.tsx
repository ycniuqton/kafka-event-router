import { useState } from 'react';
import { services } from '../../api';
import { Button, Mono, ToneBadge } from '../../components/ui';
import { Tone } from '../../domain';
import { actions, useNow, useServiceQuery } from '../../store';
import { CreateApiTokenModal } from './components/CreateApiTokenModal';
import { RevokeApiTokenModal } from './components/RevokeApiTokenModal';
import { SettingsGridRow } from './components/SettingsGridRow';
import { SettingsSection } from './components/SettingsSection';
import sectionStyles from './components/SettingsSection.module.css';
import { ApiTokenRows, ApiTokenText, type ApiTokenRow } from './model/ApiTokenRows';
import styles from './ApiTokensPanel.module.css';

/** Settings → API tokens: the caller's personal tokens (list, create shown once, revoke). */
export function ApiTokensPanel() {
  const { data, error, reload } = useServiceQuery(() => services.apiTokens.list(), []);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<ApiTokenRow | null>(null);
  const now = useNow();
  return (
    <SettingsSection title={ApiTokenText.title} hint={ApiTokenText.hint}>
      <div className={styles.header}>
        <Button size="sm" onClick={() => setCreating(true)}>{ApiTokenText.create}</Button>
      </div>
      <TokenRows rows={data ? ApiTokenRows.rows(data, now) : null} failed={Boolean(error)} onRevoke={setRevoking} />
      <CreateApiTokenModal open={creating} onClose={() => setCreating(false)} onCreated={reload} />
      <RevokeApiTokenModal
        token={revoking}
        onClose={() => setRevoking(null)}
        onRevoked={() => {
          setRevoking(null);
          actions.toast.show(ApiTokenText.revoked);
          reload();
        }}
      />
    </SettingsSection>
  );
}

/** Token rows, or a loading / empty / error line. */
function TokenRows({ rows, failed, onRevoke }: { rows: ApiTokenRow[] | null; failed: boolean; onRevoke: (row: ApiTokenRow) => void }) {
  const message = rows === null ? (failed ? ApiTokenText.unavailable : ApiTokenText.loading) : rows.length === 0 ? ApiTokenText.empty : null;
  if (message) {
    return (
      <SettingsGridRow columns={ApiTokenRows.messageColumns}>
        <span className={sectionStyles.muted}>{message}</span>
      </SettingsGridRow>
    );
  }
  return (
    <>
      {(rows ?? []).map((row) => (
        <SettingsGridRow key={row.id} columns={ApiTokenRows.columns}>
          <span>{row.name}</span>
          <Mono size="sm" truncate>{row.prefix}</Mono>
          <span className={styles.scopes}>
            {row.scopes.map((s) => <ToneBadge key={s} tone={Tone.Info}>{s}</ToneBadge>)}
          </span>
          <span className={sectionStyles.secondary}>{row.used}</span>
          <span><ToneBadge tone={row.expiryTone}>{row.expiry}</ToneBadge></span>
          <span className={sectionStyles.actions}>
            <Button size="sm" variant="ghost" onClick={() => onRevoke(row)}>{ApiTokenText.revoke}</Button>
          </span>
        </SettingsGridRow>
      ))}
    </>
  );
}
