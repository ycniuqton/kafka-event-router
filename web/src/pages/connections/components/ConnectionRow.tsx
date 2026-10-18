import { Button, EnvTag, HealthDot, Mono, TableCell, TableRow } from '../../../components/ui';
import { Can } from '../../../access';
import { ConnectionDisplay, Permission, Tone, type Connection } from '../../../domain';
import type { ConnectionRowView } from '../model/ConnectionRowModel';
import { DemoBadge } from './DemoBadge';
import styles from './ConnectionRow.module.css';

export type ConnectionRowProps = {
  row: ConnectionRowView;
  onTest: (id: string) => void;
  onBrowse: (id: string) => void;
  onEdit: (id: string) => void;
  onDelete: (connection: Connection) => void;
};

/** One saved cluster; Delete is disabled while routes use it, with a tooltip listing them; demo connections
 * carry a Demo badge and have no Edit (their cluster settings are fixed). */
export function ConnectionRow({ row, onTest, onBrowse, onEdit, onDelete }: ConnectionRowProps) {
  const { connection: c } = row;
  const inUse = row.usedByCount > 0;
  return (
    <TableRow>
      <span className={styles.name}>
        <Mono size="md" medium truncate>
          {ConnectionDisplay.name(c)}
        </Mono>
        <DemoBadge connection={c} />
      </span>
      <span className={styles.env}>
        <EnvTag env={c.env} />
      </span>
      <TableCell title={c.bootstrapServers.join('\n')}>
        <Mono tone={Tone.TextSecondary}>{row.firstServer}</Mono> <Mono tone={Tone.Muted}>{row.moreServers}</Mono>
      </TableCell>
      <span className={styles.secondary}>{c.security}</span>
      <div>
        <span className={styles.status} title={row.statusTitle}>
          <HealthDot tone={row.statusTone} label={row.statusLabel} />
        </span>
        <div className={styles.checked}>checked {row.checked}</div>
      </div>
      <span title={inUse ? row.deleteTitle : undefined}>
        <Mono tone={Tone.TextSecondary}>{row.usedByCount}</Mono>
      </span>
      <span className={styles.secondary}>{c.owner}</span>
      <div className={styles.actions}>
        <Button size="sm" onClick={() => onTest(c.id)}>
          Test
        </Button>
        <Button size="sm" onClick={() => onBrowse(c.id)} title="Browse topics">
          Topics
        </Button>
        <Can permission={Permission.ConnectionWrite}>
          {row.editable && (
            <Button size="sm" onClick={() => onEdit(c.id)}>
              Edit
            </Button>
          )}
          <span className={styles.deleteWrap} title={row.deleteTitle}>
            <Button size="sm" variant="danger" disabled={inUse} onClick={() => onDelete(c)}>
              Delete
            </Button>
          </span>
        </Can>
      </div>
    </TableRow>
  );
}
