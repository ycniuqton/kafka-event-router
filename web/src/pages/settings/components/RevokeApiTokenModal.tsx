import { useState } from 'react';
import { services } from '../../../api';
import { Button, Modal } from '../../../components/ui';
import { ApiTokenText, type ApiTokenRow } from '../model/ApiTokenRows';
import styles from '../ApiTokensPanel.module.css';

/** Confirms revoking one token; errors are shown in the dialog. */
export function RevokeApiTokenModal({ token, onClose, onRevoked }: { token: ApiTokenRow | null; onClose: () => void; onRevoked: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const revoke = async () => {
    if (!token) return;
    setBusy(true);
    setError(null);
    try {
      await services.apiTokens.revoke(token.id);
      onRevoked();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <div className={styles.footer}>
      <Button variant="secondary" onClick={onClose}>{ApiTokenText.cancel}</Button>
      <Button variant="danger" disabled={busy} onClick={revoke}>{ApiTokenText.revoke}</Button>
    </div>
  );
  return (
    <Modal open={token !== null} onClose={onClose} title={ApiTokenText.revokeTitle} subtitle={token?.name} footer={footer}>
      <p className={styles.secretNote}>{ApiTokenText.revokeBody}</p>
      {error && <p className={styles.scopeHelp}>{error}</p>}
    </Modal>
  );
}
