import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, EmptyState, EnvTag, Modal, Select } from '../../../components/ui';
import type { Connection, Team } from '../../../domain';
import { actions, useConnections, useServiceQuery } from '../../../store';
import { TeamAdmin } from '../model/TeamAdmin';
import { SettingsGridRow } from './SettingsGridRow';
import styles from './SettingsSection.module.css';

const GrantColumns = '1fr 70px 90px';

export type GrantsEditorProps = { team: Team; open: boolean; onClose: () => void; canManage: boolean };

/** Connections of other teams this team may use in its routes; admins add and revoke grants. */
export function GrantsEditor({ team, open, onClose, canManage }: GrantsEditorProps) {
  const connections = useConnections();
  const { data: grants, reload } = useServiceQuery(() => services.teams.grants(team.id), [team.id]);
  const [pick, setPick] = useState('');
  const options = TeamAdmin.grantable(team, connections, grants ?? []);
  const run = async (work: () => Promise<string>) => {
    try {
      actions.toast.show(await work());
      setPick('');
      reload();
    } catch (error) {
      actions.toast.show((error as Error).message);
    }
  };
  const chosen: Connection | undefined = options.find((c) => c.id === pick);
  return (
    <Modal open={open} onClose={onClose} title={`Connection grants · ${team.name}`} subtitle="Routes of this team may use these connections of other teams.">
      {(grants ?? []).length === 0 && <EmptyState compact title="No grants" description="This team uses only its own connections." />}
      {(grants ?? []).map((g) => (
        <SettingsGridRow key={g.connectionId} columns={GrantColumns}>
          <span className={styles.mono}>{g.connectionName}</span>
          <EnvTag env={g.envTag} />
          {canManage ? (
            <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void run(() => TeamAdmin.revoke(team, g))}>
              Revoke
            </Button>
          ) : (
            <span />
          )}
        </SettingsGridRow>
      ))}
      {canManage && (
        <div className={styles.settingRow}>
          <Select controlSize="sm" value={pick} aria-label="Connection to grant" onChange={(e) => setPick(e.target.value)}>
            <option value="">Pick a connection…</option>
            {options.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name ?? c.id} · {c.owner}
              </option>
            ))}
          </Select>
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Primary} disabled={!chosen} onClick={() => chosen && void run(() => TeamAdmin.grant(team, chosen))}>
            Grant
          </Button>
        </div>
      )}
    </Modal>
  );
}
