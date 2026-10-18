import { useState, type DragEvent } from 'react';
import { PemFiles } from '../model/PemFiles';
import styles from './PemDropZone.module.css';

export type PemDropZoneProps = {
  label: string;
  value: string;
  /** True when a value is stored server-side but never returned (private keys). */
  stored?: boolean;
  error?: string;
  onChange: (text: string) => void;
};

/** Drag-and-drop (or browse) target for one PEM file (CA, client certificate or client key); shows what is loaded. */
export function PemDropZone({ label, value, stored, error, onChange }: PemDropZoneProps) {
  const [over, setOver] = useState(false);
  const [readError, setReadError] = useState<string | null>(null);
  const accept = async (incoming: File[]) => {
    const result = await PemFiles.read(incoming);
    if ('error' in result) setReadError(result.error);
    else {
      setReadError(null);
      onChange(result.text);
    }
  };
  const onDrop = (e: DragEvent<HTMLLabelElement>) => {
    e.preventDefault();
    setOver(false);
    void accept([...e.dataTransfer.files]);
  };
  const loaded = value ? PemFiles.summary(value) : stored ? '•••••• stored' : null;
  return (
    <div className={styles.field}>
      <span className={styles.label}>{label}</span>
      {loaded ? (
        <div className={styles.loaded}>
          <span className={styles.mono}>{loaded}</span>
          <button type="button" className={styles.link} onClick={() => onChange('')} disabled={!value && !stored}>
            {value ? 'Remove' : 'Replace'}
          </button>
        </div>
      ) : (
        <label
          className={[styles.zone, over && styles.over].filter(Boolean).join(' ')}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(true);
          }}
          onDragLeave={() => setOver(false)}
          onDrop={onDrop}
        >
          <span>
            Drop a <span className={styles.mono}>.pem</span> file here
          </span>
          <span className={styles.browse}>or click to browse</span>
          <input type="file" accept={PemFiles.accept} className={styles.file} aria-label={label} onChange={(e) => void accept([...(e.target.files ?? [])])} />
        </label>
      )}
      {(readError ?? error) && <div className={styles.error}>{readError ?? error}</div>}
    </div>
  );
}
