import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonVariant, ControlSize, Field, Modal, TextArea, TextInput } from '../../../components/ui';
import { LibraryImportError, LibraryRules, type ImportIssue, type LibraryLookupTable } from '../../../domain';
import { actions } from '../../../store';
import { IssueList } from './LookupGridEditor';

/** Texts of the CSV import dialog. */
class CsvImportText {
  static readonly title = 'Import CSV';
  static readonly subtitle = 'The first line is a header with a "key" column. One more column is the value; several become an object value.';
  static readonly file = 'CSV file';
  static readonly text = 'or paste the CSV';
  static readonly note = 'Change note';
  static readonly import = 'Publish version';
  static readonly cancel = 'Cancel';
  static readonly done = (t: LibraryLookupTable) => `Published ${LibraryRules.pinLabel(t.name, t.latestVersion)} (${t.rows.toLocaleString()} rows)`;
}

type CsvImportDialogProps = { table: LibraryLookupTable; open: boolean; onClose(): void; onImported(table: LibraryLookupTable): void };

/** Uploads or pastes CSV as a new table version; per-line problems are listed and nothing is saved. */
export function CsvImportDialog({ table, open, onClose, onImported }: CsvImportDialogProps) {
  const [csv, setCsv] = useState('');
  const [note, setNote] = useState('');
  const [issues, setIssues] = useState<ImportIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setIssues([]);
    setError(null);
    try {
      const updated = await services.library.publishLookupRows(table.id, { csv }, note);
      actions.toast.show(CsvImportText.done(updated));
      setCsv('');
      onImported(updated);
    } catch (e) {
      if (e instanceof LibraryImportError) setIssues(e.issues);
      else setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <>
      <Button variant={ButtonVariant.Ghost} onClick={onClose}>{CsvImportText.cancel}</Button>
      <Button variant={ButtonVariant.Primary} disabled={busy || !csv.trim()} onClick={() => void submit()}>{CsvImportText.import}</Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title={CsvImportText.title} subtitle={CsvImportText.subtitle} footer={footer}>
      <Field label={CsvImportText.file}>
        <input type="file" accept=".csv,text/csv" aria-label={CsvImportText.file} onChange={(e) => void e.target.files?.[0]?.text().then(setCsv)} />
      </Field>
      <Field label={CsvImportText.text} error={error ?? undefined}>
        <TextArea controlSize={ControlSize.Sm} mono rows={8} value={csv} onChange={(e) => setCsv(e.target.value)} aria-label={CsvImportText.text} />
      </Field>
      <IssueList issues={issues} />
      <Field label={CsvImportText.note}>
        <TextInput controlSize={ControlSize.Sm} block value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
    </Modal>
  );
}
