import { Fragment } from 'react';
import { Drawer, DrawerWidth, Format, JsonTree } from '../../../components/ui';
import { AuditActions, type AuditEntry, type Team } from '../../../domain';
import styles from './AuditRowDetail.module.css';

export type AuditRowDetailProps = { entry: AuditEntry | null; teams: Team[]; onClose: () => void };

/** One entry in full: who, what, where from, which request, and its diff (merge patch or access details). */
export function AuditRowDetail({ entry, teams, onClose }: AuditRowDetailProps) {
  if (!entry) return null;
  const team = teams.find((t) => t.id === entry.teamId)?.name ?? entry.teamId ?? '—';
  const rows: [string, string, boolean][] = [
    ['Time', Format.monthDayTime(entry.at), true],
    ['Who', entry.actor, false],
    ['Action', `${AuditActions.label(entry.action)} (${entry.action})`, false],
    ['Category', entry.category, false],
    ['Object', `${entry.object} · ${entry.objectType.replace(/_/g, ' ')}`, false],
    ['Object id', entry.objectId || '—', true],
    ['Team', team, false],
    ['IP', entry.ip || '—', true],
    ['Request id', entry.requestId || '—', true],
  ];
  return (
    <Drawer open onClose={onClose} title={AuditActions.label(entry.action)} width={DrawerWidth.Wide}>
      <div className={styles.grid}>
        {rows.map(([key, value, mono]) => (
          <Fragment key={key}>
            <span className={styles.key}>{key}</span>
            <span className={[styles.value, mono && styles.mono].filter(Boolean).join(' ')}>{value}</span>
          </Fragment>
        ))}
      </div>
      {entry.sensitive && <p className={styles.sensitive}>Sensitive: raw data access or a credential change.</p>}
      <h3 className={styles.heading}>Diff</h3>
      {entry.diff ? <JsonTree value={entry.diff} /> : <p className={styles.empty}>No details were recorded for this action.</p>}
    </Drawer>
  );
}
