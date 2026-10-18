import { useEffect, useState } from 'react';
import { services } from '../../../../api';
import { Button, ButtonSize, ButtonVariant, Chip, ControlSize, Field, Modal, TextArea, TextInput } from '../../../../components/ui';
import styles from './SaveToLibrary.module.css';
import { LibraryRules, type LibraryTransform, type TransformStep } from '../../../../domain';
import { actions } from '../../../../store';
import { useStudio } from '../hooks/StudioContext';

/** Texts of the "Save to library" dialog. */
class SaveToLibraryText {
  static readonly open = 'Save to library';
  static readonly title = 'Save steps to the library';
  static readonly subtitle = 'Other routes can then use these steps as one library step. An existing name gets a new, immutable version.';
  static readonly name = 'Name';
  static readonly hint = 'Lower-case letters, digits and dashes, e.g. mask-customer-pii. Pick an existing name to publish a new version.';
  static readonly invalid = 'Use lower-case letters, digits and dashes.';
  static readonly description = 'Description';
  static readonly changeNote = 'Change note';
  static readonly save = 'Save';
  static readonly cancel = 'Cancel';
  static readonly maxSuggestions = 6;

  static target(existing: LibraryTransform | undefined): string {
    return existing ? `New version ${LibraryRules.versionLabel(existing.latestVersion + 1)} of ${existing.name}` : 'New library transform (v1)';
  }

  static saved(name: string, version: string): string {
    return `Saved ${name} ${version} to the library`;
  }
}

/** Studio action: saves the enabled steps as a library transform (new transform or new version). */
export function SaveToLibrary() {
  const { model } = useStudio();
  const steps = model.steps.filter((s) => s.enabled);
  const flow = useSaveToLibrary(steps);
  if (model.readOnly || !steps.length) return null;
  const { form, edit } = flow;
  const footer = (
    <>
      <Button variant={ButtonVariant.Ghost} onClick={() => flow.setOpen(false)}>{SaveToLibraryText.cancel}</Button>
      <Button variant={ButtonVariant.Primary} disabled={flow.saving || !form.name} onClick={() => void flow.save()}>{SaveToLibraryText.save}</Button>
    </>
  );
  return (
    <>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => flow.setOpen(true)}>{SaveToLibraryText.open}</Button>
      <Modal open={flow.open} onClose={() => flow.setOpen(false)} title={SaveToLibraryText.title} subtitle={SaveToLibraryText.subtitle} footer={footer}>
        <Field label={SaveToLibraryText.name} hint={form.name ? SaveToLibraryText.target(flow.target) : SaveToLibraryText.hint} error={flow.error ?? undefined}>
          <TextInput controlSize={ControlSize.Md} mono block autoFocus value={form.name} onChange={(e) => edit({ name: e.target.value })} />
        </Field>
        <div className={styles.names}>
          {flow.suggestions.map((t) => <Chip key={t.id} onClick={() => edit({ name: t.name })}>{t.name}</Chip>)}
        </div>
        {!flow.target && (
          <Field label={SaveToLibraryText.description}>
            <TextInput controlSize={ControlSize.Md} block value={form.description} onChange={(e) => edit({ description: e.target.value })} />
          </Field>
        )}
        <Field label={SaveToLibraryText.changeNote}>
          <TextArea controlSize={ControlSize.Sm} rows={2} value={form.changeNote} onChange={(e) => edit({ changeNote: e.target.value })} />
        </Field>
      </Modal>
    </>
  );
}

/** Form state of "Save to library": name (with existing-name suggestions), description, change note, save. */
function useSaveToLibrary(steps: TransformStep[]) {
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', description: '', changeNote: '' });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const existing = useExistingTransforms(open);
  const save = async () => {
    if (!LibraryRules.validName(form.name)) return setError(SaveToLibraryText.invalid);
    setSaving(true);
    try {
      const saved = await services.library.saveTransform(form.name, steps, { description: form.description, changeNote: form.changeNote });
      actions.toast.show(SaveToLibraryText.saved(saved.name, saved.version));
      setOpen(false);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const edit = (patch: Partial<typeof form>) => {
    setForm({ ...form, ...patch });
    setError(null);
  };
  const suggestions = existing.filter((t) => t.name !== form.name && t.name.includes(form.name)).slice(0, SaveToLibraryText.maxSuggestions);
  return { open, setOpen, form, edit, error, saving, save, suggestions, target: existing.find((t) => t.name === form.name) };
}

/** Library transforms (name suggestions and the "new version" hint), loaded when the dialog opens. */
function useExistingTransforms(open: boolean): LibraryTransform[] {
  const [items, setItems] = useState<LibraryTransform[]>([]);
  useEffect(() => {
    if (!open) return;
    let live = true;
    services.library.list().then((list) => live && setItems(list), () => undefined);
    return () => {
      live = false;
    };
  }, [open]);
  return items;
}
