import { Button, Modal } from '../../../components/ui';
import type { Route } from '../../../domain';

type ArchiveRouteDialogProps = { route: Route | undefined; onCancel: () => void; onConfirm: () => void };

/** Confirms Archive: the route stops and disappears from the list (there is no hard delete). */
export function ArchiveRouteDialog({ route, onCancel, onConfirm }: ArchiveRouteDialogProps) {
  const footer = (
    <>
      <Button size="lg" onClick={onCancel}>Cancel</Button>
      <Button size="lg" variant="danger" onClick={onConfirm}>Archive route</Button>
    </>
  );
  return (
    <Modal open={!!route} onClose={onCancel} title={`Archive “${route?.name ?? ''}”?`} footer={footer} hideClose>
      The route stops reading and is hidden from the list. Its versions and history are kept.
    </Modal>
  );
}
