import { ConnectionDisplay, type Connection } from '../../../domain';
import { ConnectionPanelMode, type ConnectionPanel } from '../model/ConnectionDrawer';
import type { ConnectionTestDrawerState } from '../model/useConnectionTestDrawer';
import { ConnectionFormDrawer } from './ConnectionFormDrawer';
import { ConnectionTestDrawer } from './ConnectionTestDrawer';
import { TopicBrowserDrawer } from '../topics/TopicBrowserDrawer';

export type ConnectionOverlaysProps = {
  connections: Connection[];
  panel: ConnectionPanel | null;
  tests: ConnectionTestDrawerState;
  onClosePanel: () => void;
  onSaved: (connectionId: string) => void;
};

/** The one side panel open at a time: test checklist, topic browser or create / edit form. */
export function ConnectionOverlays({ connections, panel, tests, onClosePanel, onSaved }: ConnectionOverlaysProps) {
  const panelConnection = connections.find((c) => c.id === panel?.connectionId);
  const testConnection = connections.find((c) => c.id === tests.testId);
  return (
    <>
      {testConnection && <ConnectionTestDrawer connection={testConnection} onClose={tests.closeTest} onRetest={tests.retest} />}
      {panel?.mode === ConnectionPanelMode.Browse && panelConnection && <TopicBrowserDrawer connection={panelConnection} onClose={onClosePanel} />}
      {panel && panel.mode !== ConnectionPanelMode.Browse && (
        <ConnectionFormDrawer
          key={`${panel.mode}:${panel.connectionId ?? ''}`}
          editing={panelConnection ?? null}
          takenNames={connections.map((c) => ConnectionDisplay.name(c))}
          onClose={onClosePanel}
          onSaved={onSaved}
        />
      )}
    </>
  );
}
