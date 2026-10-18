import { useEffect, useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, ControlSize, Mono, Tabs, TextInput } from '../../../components/ui';
import { LibraryImportError, LibraryKind, LibraryRules, LookupKeyTypeLabel, Tone, type ImportIssue, type LibraryLookupTable } from '../../../domain';
import { actions } from '../../../store';
import { DetailTab, LibraryText } from '../model/LibraryLayout';
import { LookupGridModel } from '../model/LookupGridModel';
import { CsvImportDialog } from './CsvImportDialog';
import { IssueList, LookupGridEditor } from './LookupGridEditor';
import { LookupRowsView } from './LookupRowsView';
import { MetadataSection } from './MetadataSection';
import { UsageList } from './UsageList';
import { VersionList } from './VersionList';
import styles from './LibraryList.module.css';

/** Texts of the table detail. */
class TableText {
  static readonly edit = 'Edit rows';
  static readonly import = 'Import CSV';
  static readonly publish = 'Publish version';
  static readonly cancel = 'Cancel';
  static readonly note = 'Change note';
  static readonly tooMany = 'This table is too large to edit here; import a CSV instead.';
  static readonly editLimit = 2000;
  static readonly published = (t: LibraryLookupTable) => `Published ${LibraryRules.pinLabel(t.name, t.latestVersion)}`;
}

type LookupTableDetailProps = { table: LibraryLookupTable; onChange(t: LibraryLookupTable): void; onArchived(): void };

/** Drawer body of a lookup table: metadata, rows (paged), grid edit or CSV import as a new version, versions, usages. */
export function LookupTableDetail({ table, onChange, onArchived }: LookupTableDetailProps) {
  const [tab, setTab] = useState<DetailTab>(DetailTab.Rows);
  const [importing, setImporting] = useState(false);
  const [editing, setEditing] = useState(false);
  const changed = (t: LibraryLookupTable) => {
    onChange(t);
    setImporting(false);
    setEditing(false);
  };
  return (
    <div className={styles.detail}>
      <div className={styles.meta}>
        <Mono tone={Tone.Accent}>{table.version}</Mono>
        <span>{LibraryText.rowsCount(table.rows)}</span>
        <span>{LookupKeyTypeLabel[table.keyType]} keys</span>
        <span>{LibraryRules.usageLabel(table.usedBy)}</span>
        <span className={styles.muted}>{table.team}</span>
      </div>
      <MetadataSection kind={LibraryKind.LookupTable} item={table} onChange={onChange} onArchived={onArchived} />
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} disabled={table.rows > TableText.editLimit} title={table.rows > TableText.editLimit ? TableText.tooMany : undefined} onClick={() => setEditing(true)}>{TableText.edit}</Button>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setImporting(true)}>{TableText.import}</Button>
      </div>
      {editing && <GridPublisher table={table} onDone={changed} onCancel={() => setEditing(false)} />}
      <Tabs items={LibraryText.tableTabs} value={tab} onChange={setTab} />
      {tab === DetailTab.Rows && <LookupRowsView key={table.latestVersion} id={table.id} version={table.latestVersion} />}
      {tab === DetailTab.Versions && <VersionList kind={LibraryKind.LookupTable} id={table.id} refresh={table.latestVersion} />}
      {tab === DetailTab.Usages && <UsageList kind={LibraryKind.LookupTable} id={table.id} />}
      <CsvImportDialog table={table} open={importing} onClose={() => setImporting(false)} onImported={changed} />
    </div>
  );
}

/** Loads the latest rows into the grid editor and publishes the edited grid as a new version. */
function GridPublisher({ table, onDone, onCancel }: { table: LibraryLookupTable; onDone(t: LibraryLookupTable): void; onCancel(): void }) {
  const [grid, setGrid] = useState<LookupGridModel | null>(null);
  const [note, setNote] = useState('');
  const [issues, setIssues] = useState<ImportIssue[]>([]);
  useEffect(() => {
    let live = true;
    GridLoader.load(table).then((g) => live && setGrid(g), () => undefined);
    return () => {
      live = false;
    };
  }, [table]);
  if (!grid) return null;
  const publish = async () => {
    try {
      const updated = await services.library.publishLookupRows(table.id, { rows: grid.toRows() }, note);
      actions.toast.show(TableText.published(updated));
      onDone(updated);
    } catch (e) {
      setIssues(e instanceof LibraryImportError ? e.issues : [{ line: 0, message: (e as Error).message }]);
    }
  };
  return (
    <div className={styles.detail}>
      <LookupGridEditor grid={grid} onChange={setGrid} />
      <IssueList issues={issues} />
      <div className={styles.actions}>
        <TextInput controlSize={ControlSize.Sm} className={styles.grow} placeholder={TableText.note} aria-label={TableText.note} value={note} onChange={(e) => setNote(e.target.value)} />
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Primary} disabled={grid.issues().length > 0} onClick={() => void publish()}>{TableText.publish}</Button>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={onCancel}>{TableText.cancel}</Button>
      </div>
    </div>
  );
}

/** Reads every row of the latest version page by page (tables up to the edit limit). */
class GridLoader {
  static async load(table: LibraryLookupTable): Promise<LookupGridModel> {
    const rows = [];
    let cursor: string | null = null;
    do {
      const page = await services.library.rows(table.id, table.latestVersion, cursor);
      rows.push(...page.rows);
      cursor = page.next;
    } while (cursor);
    return LookupGridModel.fromRows(rows);
  }
}
