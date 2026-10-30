import { Field, TextArea } from '../../../components/ui';
import { EditField, type RecordEditTexts } from '../../../domain';
import styles from './ReplayDialog.module.css';

/** Labels and sizes of the editor fields. */
const EditorFields: { field: EditField; label: string; rows: number; hint: string }[] = [
  { field: EditField.Value, label: 'Value', rows: 12, hint: 'JSON' },
  { field: EditField.Key, label: 'Key', rows: 2, hint: 'JSON (a string key is "quoted")' },
  { field: EditField.Headers, label: 'Headers', rows: 4, hint: 'JSON array of {"key", "value"}' },
];

/** Masked editors only ever see masks; the server keeps the real value of every mask left untouched. */
export const MaskedEditNotice = "Masked values you don't change are kept as they are.";

export type RecordEditorProps = {
  texts: RecordEditTexts;
  errors: Partial<Record<EditField, string>>;
  masked: boolean;
  disabled: boolean;
  onType: (field: EditField, text: string) => void;
};

/** JSON editor of a dead letter's value, key and headers with live validation. */
export function RecordEditor({ texts, errors, masked, disabled, onType }: RecordEditorProps) {
  return (
    <div className={styles.editor}>
      {masked && <div className={styles.masked}>{MaskedEditNotice}</div>}
      {EditorFields.map(({ field, label, rows, hint }) => (
        <Field key={field} label={label} hint={hint} error={errors[field]}>
          <TextArea mono rows={rows} spellCheck={false} value={texts[field]} disabled={disabled} aria-invalid={!!errors[field]} onChange={(e) => onType(field, e.target.value)} />
        </Field>
      ))}
    </div>
  );
}
