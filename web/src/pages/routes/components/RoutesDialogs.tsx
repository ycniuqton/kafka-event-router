import type { Route } from '../../../domain';
import type { BulkRouteActions } from '../model/BulkRouteActions';
import type { RouteCommands } from '../model/RouteCommands';
import { RoutesDialog } from '../model/RoutesDialog';
import { AddTagModal } from './AddTagModal';
import { ArchiveRouteDialog } from './ArchiveRouteDialog';
import { ImportRoutesDialog } from './ImportRoutesDialog';

type RoutesDialogsProps = {
  open: RoutesDialog;
  archiving: Route | undefined;
  selected: number;
  bulk: BulkRouteActions;
  commands: RouteCommands;
  onClose: () => void;
  onArchiveDone: () => void;
};

/** Archive confirmation, Add tag and Import dialogs of the routes list. */
export function RoutesDialogs({ open, archiving, selected, bulk, commands, onClose, onArchiveDone }: RoutesDialogsProps) {
  return (
    <>
      <ArchiveRouteDialog
        route={archiving}
        onCancel={onArchiveDone}
        onConfirm={() => {
          if (archiving) commands.archiveNow(archiving.id);
          onArchiveDone();
        }}
      />
      <AddTagModal
        open={open === RoutesDialog.Tag}
        count={selected}
        onClose={onClose}
        onConfirm={(tag) => {
          onClose();
          bulk.addTag(tag);
        }}
      />
      <ImportRoutesDialog open={open === RoutesDialog.Import} onClose={onClose} />
    </>
  );
}
