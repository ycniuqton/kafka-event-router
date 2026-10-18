import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, ControlSize, Field, TextArea } from '../../../components/ui';
import type { LibraryKind, LibraryTransform } from '../../../domain';
import { actions } from '../../../store';
import { LibraryText } from '../model/LibraryLayout';
import styles from './LibraryList.module.css';

type MetadataSectionProps<T extends LibraryTransform> = { kind: LibraryKind; item: T; onChange(item: T): void; onArchived(): void };

/** Description (If-Match save) and archive (refused with a message while routes use the object). */
export function MetadataSection<T extends LibraryTransform>({ kind, item, onChange, onArchived }: MetadataSectionProps<T>) {
  const [description, setDescription] = useState(item.description);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async (work: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const save = () => run(async () => {
    onChange(await services.library.updateDescription(kind, item, description));
    actions.toast.show(LibraryText.descriptionSaved);
  });
  const archive = () => run(async () => {
    await services.library.archive(kind, item.id);
    actions.toast.show(LibraryText.archived(item.name));
    onArchived();
  });
  return (
    <div className={styles.detail}>
      <Field label={LibraryText.description}>
        <TextArea controlSize={ControlSize.Sm} rows={2} value={description} onChange={(e) => setDescription(e.target.value)} aria-label={LibraryText.description} />
      </Field>
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} disabled={busy || description === item.description} onClick={() => void save()}>{LibraryText.saveDescription}</Button>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Danger} disabled={busy} onClick={() => void archive()}>{LibraryText.archive}</Button>
      </div>
      {error && <div className={styles.error} role="alert">{error}</div>}
    </div>
  );
}
