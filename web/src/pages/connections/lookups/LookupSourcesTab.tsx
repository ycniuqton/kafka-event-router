import { useMemo, useState } from 'react';
import { services } from '../../../api';
import { Button, Chip, Drawer, Modal, Mono, TextInput } from '../../../components/ui';
import { Can } from '../../../access';
import { LookupSourceInUseError, LookupSourceKindMeta, Permission, type LookupSource, type LookupTestResult } from '../../../domain';
import { actions, useNow, useServiceQuery } from '../../../store';
import { LookupSourceFormDrawer } from './LookupSourceFormDrawer';
import { LookupSourcesTable } from './LookupSourcesTable';
import { LookupTestChecklist } from './LookupTestChecklist';
import { LookupSourceRows, type LookupSourceFilterState } from './model/LookupSourceRows';
import type { LookupSaved } from './model/useLookupSourceForm';
import styles from './LookupSources.module.css';

/** Texts of the tab. */
const LookupTabText = {
  create: '+ New lookup source',
  search: 'Search name or description',
  all: 'All',
  saved: (name: string) => `Lookup source ${name} saved.`,
  deleted: (name: string) => `Lookup source ${name} deleted.`,
  testTitle: (name: string) => `Test ${name}`,
  inUse: (names: string) => `Routes still use this source: ${names}.`,
} as const;

/** Which panel is open: the form (create when `editing` is null) or a test result. */
type LookupPanel = { form: true; editing: LookupSource | null } | { form: false; testing: LookupSource; result: LookupTestResult | null; failure: string | null };

/** Connections → Lookup sources (LS-01..LS-12): filters, table, form drawer, test drawer, delete confirm. */
export function LookupSourcesTab() {
  const { data, reload } = useServiceQuery(() => services.lookupSources.list(), []);
  const now = useNow();
  const [filter, setFilter] = useState<LookupSourceFilterState>(LookupSourceRows.initialFilter);
  const [panel, setPanel] = useState<LookupPanel | null>(null);
  const [deleting, setDeleting] = useState<LookupSource | null>(null);
  const sources = useMemo(() => data ?? [], [data]);
  const rows = useMemo(() => LookupSourceRows.filter(sources, filter).map((s) => LookupSourceRows.build(s, now)), [sources, filter, now]);
  const test = async (source: LookupSource) => {
    setPanel({ form: false, testing: source, result: null, failure: null });
    const out = await services.lookupSources.test(source.id).then((result) => ({ result, failure: null }), (e: Error) => ({ result: null, failure: e.message }));
    setPanel({ form: false, testing: source, ...out });
    reload();
  };
  const onSaved = ({ source, test: result }: LookupSaved) => {
    actions.toast.show(LookupTabText.saved(source.name));
    setPanel({ form: false, testing: source, result, failure: null });
    reload();
  };
  return (
    <>
      <LookupSourceFilters value={filter} onChange={setFilter} onCreate={() => setPanel({ form: true, editing: null })} />
      <LookupSourcesTable rows={rows} filtered={!!filter.kind || !!filter.text.trim()} onCreate={() => setPanel({ form: true, editing: null })}
        onTest={(s) => void test(s)} onEdit={(s) => setPanel({ form: true, editing: s })} onDelete={setDeleting} />
      {panel?.form && <LookupSourceFormDrawer editing={panel.editing} onClose={() => setPanel(null)} onSaved={onSaved} />}
      {panel && !panel.form && (
        <Drawer open onClose={() => setPanel(null)} title={LookupTabText.testTitle(panel.testing.name)}>
          <LookupTestChecklist result={panel.result} failure={panel.failure} />
        </Drawer>
      )}
      <DeleteLookupSourceModal source={deleting} onClose={() => setDeleting(null)} onDeleted={reload} />
    </>
  );
}

/** Search, kind chips and the create button above the table. */
function LookupSourceFilters({ value, onChange, onCreate }: { value: LookupSourceFilterState; onChange: (v: LookupSourceFilterState) => void; onCreate: () => void }) {
  return (
    <div className={styles.bar}>
      <TextInput className={styles.search} controlSize="md" sunken value={value.text} placeholder={LookupTabText.search} aria-label={LookupTabText.search} onChange={(e) => onChange({ ...value, text: e.target.value })} />
      <div className={styles.chips}>
        <Chip active={!value.kind} onClick={() => onChange({ ...value, kind: null })}>{LookupTabText.all}</Chip>
        {LookupSourceRows.kinds().map((k) => <Chip key={k} active={value.kind === k} onClick={() => onChange({ ...value, kind: k })}>{LookupSourceKindMeta[k].label}</Chip>)}
      </div>
      <span className={styles.spacer} />
      <Can permission={Permission.ConnectionWrite}><Button onClick={onCreate}>{LookupTabText.create}</Button></Can>
    </div>
  );
}

/** Confirms deleting an unused source; a route that started using it meanwhile makes the server refuse (toast lists routes). */
function DeleteLookupSourceModal({ source, onClose, onDeleted }: { source: LookupSource | null; onClose: () => void; onDeleted: () => void }) {
  const [busy, setBusy] = useState(false);
  const confirm = async () => {
    if (!source) return;
    setBusy(true);
    try {
      await services.lookupSources.remove(source.id);
      actions.toast.show(LookupTabText.deleted(source.name));
      onDeleted();
    } catch (error) {
      const message = error instanceof LookupSourceInUseError ? LookupTabText.inUse(error.routes.map((r) => r.name).join(LookupSourceRows.separator)) : (error as Error).message;
      actions.toast.show(message);
    } finally {
      setBusy(false);
      onClose();
    }
  };
  const footer = (
    <>
      <Button size="lg" onClick={onClose}>Cancel</Button>
      <Button size="lg" variant="danger" disabled={busy} onClick={() => void confirm()}>Delete</Button>
    </>
  );
  return (
    <Modal open={!!source} onClose={onClose} title="Delete lookup source" subtitle="No route uses it. Credentials stored for it are removed too." footer={footer}>
      <div>Delete <Mono size="md">{source?.name ?? ''}</Mono>? This can't be undone.</div>
    </Modal>
  );
}
