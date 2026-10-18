import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonVariant, ControlSize, Field, Modal, Select, TextArea, TextInput } from '../../../components/ui';
import { LibraryRules, LookupCsv, LookupKeyType, LookupKeyTypeLabel, type ImportIssue, type LibraryLookupTable } from '../../../domain';
import { actions } from '../../../store';
import { IssueList } from './LookupGridEditor';

/** Texts of the new-table dialog. */
class NewTableText {
  static readonly title = 'New lookup table';
  static readonly name = 'Name';
  static readonly keyType = 'Key type';
  static readonly description = 'Description';
  static readonly rows = 'Rows (CSV with a key column)';
  static readonly placeholder = 'key,value\nDE,EMEA\nVN,APAC';
  static readonly create = 'Create';
  static readonly cancel = 'Cancel';
  static readonly invalidName = 'Use lower-case letters, digits and dashes.';
  static readonly created = (t: LibraryLookupTable) => `Created ${LibraryRules.pinLabel(t.name, t.latestVersion)}`;
}

/** Creates a named lookup table with its first version from CSV text. */
export function NewLookupTableDialog({ open, onClose, onCreated }: { open: boolean; onClose(): void; onCreated(t: LibraryLookupTable): void }) {
  const [form, setForm] = useState({ name: '', description: '', keyType: LookupKeyType.String as LookupKeyType, csv: '' });
  const [issues, setIssues] = useState<ImportIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const create = async () => {
    if (!LibraryRules.validName(form.name)) return setError(NewTableText.invalidName);
    const parsed = LookupCsv.parse(form.csv);
    setIssues(parsed.issues);
    if (parsed.issues.length) return;
    try {
      const table = await services.library.createLookupTable({ name: form.name, description: form.description, keyType: form.keyType, rows: parsed.rows, changeNote: '' });
      actions.toast.show(NewTableText.created(table));
      onCreated(table);
    } catch (e) {
      setError((e as Error).message);
    }
  };
  const edit = (patch: Partial<typeof form>) => {
    setForm({ ...form, ...patch });
    setError(null);
  };
  const footer = (
    <>
      <Button variant={ButtonVariant.Ghost} onClick={onClose}>{NewTableText.cancel}</Button>
      <Button variant={ButtonVariant.Primary} disabled={!form.name || !form.csv.trim()} onClick={() => void create()}>{NewTableText.create}</Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title={NewTableText.title} footer={footer}>
      <Field label={NewTableText.name} error={error ?? undefined}>
        <TextInput controlSize={ControlSize.Md} mono block autoFocus value={form.name} onChange={(e) => edit({ name: e.target.value })} />
      </Field>
      <Field label={NewTableText.keyType}>
        <Select controlSize={ControlSize.Md} value={form.keyType} onChange={(e) => edit({ keyType: e.target.value as LookupKeyType })}>
          {Object.values(LookupKeyType).map((t) => <option key={t} value={t}>{LookupKeyTypeLabel[t]}</option>)}
        </Select>
      </Field>
      <Field label={NewTableText.description}>
        <TextInput controlSize={ControlSize.Md} block value={form.description} onChange={(e) => edit({ description: e.target.value })} />
      </Field>
      <Field label={NewTableText.rows}>
        <TextArea controlSize={ControlSize.Sm} mono rows={6} placeholder={NewTableText.placeholder} value={form.csv} onChange={(e) => edit({ csv: e.target.value })} aria-label={NewTableText.rows} />
      </Field>
      <IssueList issues={issues} />
    </Modal>
  );
}
