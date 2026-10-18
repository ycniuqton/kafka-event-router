import { useState } from 'react';
import { Button, Modal, Mono } from '../../../components/ui';
import { ConnectionDisplay, type Connection } from '../../../domain';
import { actions } from '../../../store';

export type DeleteConnectionModalProps = { connection: Connection | null; onClose: () => void };

/** Confirms deleting an unused connection; the server refuses (toast lists routes) if a route started using it meanwhile. */
export function DeleteConnectionModal({ connection, onClose }: DeleteConnectionModalProps) {
  const [busy, setBusy] = useState(false);
  const confirm = async () => {
    setBusy(true);
    try {
      if (connection) await actions.connections.remove(connection.id);
    } catch (error) {
      actions.toast.show((error as Error).message);
    } finally {
      setBusy(false);
      onClose();
    }
  };
  return (
    <Modal
      open={!!connection}
      onClose={onClose}
      title="Delete connection"
      subtitle="No route uses it. Credentials stored for it are removed too."
      footer={
        <>
          <Button size="lg" onClick={onClose}>
            Cancel
          </Button>
          <Button size="lg" variant="danger" disabled={busy} onClick={() => void confirm()}>
            Delete
          </Button>
        </>
      }
    >
      <div>
        Delete <Mono size="md">{connection ? ConnectionDisplay.name(connection) : ''}</Mono>? This can't be undone.
      </div>
    </Modal>
  );
}
