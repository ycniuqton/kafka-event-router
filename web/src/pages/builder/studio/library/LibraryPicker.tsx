import { useEffect, useState } from 'react';
import { services } from '../../../../api';
import { ControlSize, Select } from '../../../../components/ui';
import { LibraryKind, LibraryRules, type LibraryTransform } from '../../../../domain';
import { LibraryPickerText } from './LibraryPin';
import styles from './Library.module.css';


type LibraryPickerProps = { kind: LibraryKind; name: string; version: number | undefined; onChange(name: string, version: number): void };

/** Select of a library object (by name) and one of its versions; picking an object pins its latest version. */
export function LibraryPicker({ kind, name, version, onChange }: LibraryPickerProps) {
  const [items, setItems] = useState<LibraryTransform[] | null>(null);
  useEffect(() => {
    let live = true;
    const load = kind === LibraryKind.Transform ? services.library.list() : services.library.lookupTables();
    load.then((list) => live && setItems(list), () => live && setItems([]));
    return () => {
      live = false;
    };
  }, [kind]);
  const current = items?.find((i) => i.name === name);
  const latest = current?.latestVersion ?? version ?? 0;
  const versions = Array.from({ length: latest }, (_, i) => latest - i);
  return (
    <div className={styles.row}>
      <Select controlSize={ControlSize.Sm} mono sunken className={styles.grow} value={name} aria-label={LibraryPickerText.object}
        onChange={(e) => {
          const picked = items?.find((i) => i.name === e.target.value);
          if (picked) onChange(picked.name, picked.latestVersion);
        }}>
        <option value="">{items && !items.length ? LibraryPickerText.empty : LibraryPickerText.choose}</option>
        {name && !current && <option value={name}>{name}</option>}
        {(items ?? []).map((i) => (
          <option key={i.id} value={i.name}>
            {i.name}
          </option>
        ))}
      </Select>
      <Select controlSize={ControlSize.Sm} mono sunken value={version ?? ''} aria-label={LibraryPickerText.version} disabled={!name}
        onChange={(e) => onChange(name, Number(e.target.value))}>
        {versions.map((v) => (
          <option key={v} value={v}>
            {LibraryRules.versionLabel(v)}
          </option>
        ))}
      </Select>
    </div>
  );
}
