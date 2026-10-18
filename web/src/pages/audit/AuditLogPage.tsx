import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { services } from '../../api';
import { Can } from '../../access';
import { EmptyState, Page, PageHeader, PageWidth } from '../../components/ui';
import { Permission, type AuditEntry, type AuditQuery } from '../../domain';
import { useServiceQuery } from '../../store';
import { AuditFilterBar } from './components/AuditFilterBar';
import { AuditRowDetail } from './components/AuditRowDetail';
import { AuditTable } from './components/AuditTable';
import { ExportButton } from './components/ExportButton';
import { AuditFilter, type AuditFilterState } from './model/AuditFilter';
import { AuditPresetParam, AuditPresets } from './model/AuditPresets';
import { useAuditLog } from './model/useAuditLog';
import styles from './AuditPage.module.css';

/** The bar state and the server query it resolved to (relative ranges are fixed when the filter changes). */
type FilterSelection = { filter: AuditFilterState; query: AuditQuery };

/** Audit log (spec §11.3): every action, read-only, filtered on the server; workspace admins only. */
export default function AuditLogPage() {
  const [params] = useSearchParams();
  const [selection, setSelection] = useState<FilterSelection>(() => AuditLogSelection.of(AuditPresets.initial(params.get(AuditPresetParam))));
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Audit}>
        <PageHeader title="Audit log" subtitle="Every action, read-only. Raw-sample access is included."
          actions={<Can permission={Permission.AuditExport}><ExportButton query={selection.query} /></Can>} />
        <Can permission={Permission.AuditRead} fallback={<EmptyState title="Only workspace admins can read the audit log." />}>
          <AuditLog selection={selection} onChange={(filter) => setSelection(AuditLogSelection.of(filter))} />
        </Can>
      </Page>
    </div>
  );
}

/** Resolves bar states into server queries against the services clock. */
class AuditLogSelection {
  static of(filter: AuditFilterState): FilterSelection {
    return { filter, query: AuditFilter.toQuery(filter, services.clock.now()) };
  }
}

function AuditLog({ selection, onChange }: { selection: FilterSelection; onChange: (filter: AuditFilterState) => void }) {
  const { data: actors } = useServiceQuery(() => services.audit.actors(), []);
  const { data: actions } = useServiceQuery(() => services.audit.actions(), []);
  const { data: teams } = useServiceQuery(() => services.teams.list(), []);
  const [selected, setSelected] = useState<AuditEntry | null>(null);
  const log = useAuditLog(selection.query);
  const teamList = useMemo(() => teams ?? [], [teams]);
  return (
    <>
      <AuditFilterBar filter={selection.filter} actors={actors ?? []} actions={actions ?? []} teams={teamList} onChange={onChange} />
      <AuditTable entries={log.entries} loading={log.loading} error={log.error} hasMore={log.hasMore} filtered={AuditFilter.isFiltered(selection.filter)}
        onLoadMore={log.loadMore} onReset={() => onChange(AuditFilter.initial)} onSelect={setSelected} />
      <AuditRowDetail entry={selected} teams={teamList} onClose={() => setSelected(null)} />
    </>
  );
}
