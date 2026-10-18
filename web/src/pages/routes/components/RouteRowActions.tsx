import type { MouseEvent } from 'react';
import { Button } from '../../../components/ui';
import { useCan } from '../../../access';
import { Permission, RouteRules, RouteStatus, type Route } from '../../../domain';
import type { RouteCommands } from '../model/RouteCommands';
import styles from './RouteRowActions.module.css';

type RouteRowActionsProps = { route: Route; commands: RouteCommands; compact?: boolean };
type Run = (command: (id: string) => void) => (e: MouseEvent) => void;

/** Pause/Resume, Edit, View metrics, Export, Duplicate and Archive (archived rows: Unarchive, Export); clicks never open the row. */
export function RouteRowActions({ route, commands, compact }: RouteRowActionsProps) {
  const canWrite = useCan(Permission.RouteWrite);
  const run: Run = (command) => (e) => {
    e.stopPropagation();
    command.call(commands, route.id);
  };
  if (route.status === RouteStatus.Archived) return <ArchivedRowActions canWrite={canWrite} commands={commands} run={run} />;
  return (
    <div className={styles.actions}>
      {canWrite && RouteRules.canPause(route.status) && (
        <Button size="sm" onClick={run(commands.togglePause)}>
          {RouteRules.pauseLabel(route.status)}
        </Button>
      )}
      {!compact && (
        <>
          <Button size="sm" onClick={run(commands.edit)}>Edit</Button>
          <IconAction title="View metrics" icon="▤" onClick={run(commands.viewMetrics)} />
          <IconAction title="Export YAML" icon="⇩" onClick={run(commands.exportYaml)} />
          {canWrite && <IconAction title="Duplicate" icon="⧉" onClick={run(commands.duplicate)} />}
          {canWrite && <IconAction title="Archive" icon="⌫" onClick={run(commands.archive)} />}
        </>
      )}
    </div>
  );
}

/** Archived rows: Unarchive (writers) and Export. */
function ArchivedRowActions({ canWrite, commands, run }: { canWrite: boolean; commands: RouteCommands; run: Run }) {
  return (
    <div className={styles.actions}>
      {canWrite && <Button size="sm" onClick={run(commands.unarchive)}>Unarchive</Button>}
      <IconAction title="Export YAML" icon="⇩" onClick={run(commands.exportYaml)} />
    </div>
  );
}

/** Square icon button with its title as accessible name. */
function IconAction({ title, icon, onClick }: { title: string; icon: string; onClick: (e: MouseEvent) => void }) {
  return (
    <Button size="sm" square className={styles.icon} title={title} aria-label={title} onClick={onClick}>
      {icon}
    </Button>
  );
}
