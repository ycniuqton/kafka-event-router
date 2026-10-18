import { useState } from 'react';
import { PendingApprovalBanner } from '../../../access';
import { Card, EmptyState } from '../../../components/ui';
import type { Route } from '../../../domain';
import { actions, useVersions } from '../../../store';
import { CompareSelection } from '../model/VersionCompare';
import { VersionPresentation } from '../model/VersionPresentation';
import { HistoryToolbar } from './HistoryToolbar';
import { RollbackDialog } from './RollbackDialog';
import { VersionChanges } from './VersionChanges';
import { VersionDiffView } from './VersionDiffView';
import { VersionRow } from './VersionRow';
import styles from './HistoryTab.module.css';

/** Version history (spec §11.1): select one to see its changes, tick two to compare, roll back. */
export function HistoryTab({ route }: { route: Route }) {
  const versions = useVersions(route.id);
  const [selected, setSelected] = useState<number>();
  const [picked, setPicked] = useState<number[]>([]);
  const [rollbackTo, setRollbackTo] = useState<number>();
  if (!versions) return null;
  if (!versions.length) return <EmptyState compact title="No versions yet" description="Every deploy creates a version you can compare and roll back to." />;
  const current = VersionPresentation.current(versions);
  const shown = versions.find((v) => v.version === selected) ?? current ?? versions[0];
  const confirmRollback = () => {
    if (rollbackTo !== undefined) void actions.routes.rollback(route.id, rollbackTo).then(() => setSelected(undefined));
    setRollbackTo(undefined);
  };
  return (
    <div className={styles.tab}>
      <PendingApprovalBanner routeId={route.id} />
      <HistoryToolbar />
      <div className={styles.split}>
        <Card>
          {versions.map((v) => (
            <VersionRow
              key={v.version}
              version={v}
              current={v === current}
              selected={v === shown}
              compared={picked.includes(v.version)}
              onSelect={() => setSelected(v.version)}
              onCompare={() => setPicked((p) => CompareSelection.toggle(p, v.version))}
              onRollback={() => setRollbackTo(v.version)}
            />
          ))}
        </Card>
        {picked.length === CompareSelection.limit ? (
          <VersionDiffView routeId={route.id} picked={[picked[0], picked[1]]} onClear={() => setPicked([])} />
        ) : (
          <VersionChanges version={shown} />
        )}
      </div>
      <RollbackDialog version={rollbackTo} nextVersion={versions[0].version + 1} onCancel={() => setRollbackTo(undefined)} onConfirm={confirmRollback} />
    </div>
  );
}
