import { useState } from 'react';
import { services } from '../../../api';
import { Button, Select } from '../../../components/ui';
import { AuditFileFormat, type AuditQuery } from '../../../domain';
import { actions } from '../../../store';
import styles from './ExportButton.module.css';

/** Format labels of the export select. */
const FormatLabel: Record<AuditFileFormat, string> = { [AuditFileFormat.Csv]: 'CSV', [AuditFileFormat.Jsonl]: 'JSONL' };

/** Exports the log with the current filters (JSONL or CSV); the server streams the file and audits the export. */
export function ExportButton({ query }: { query: AuditQuery }) {
  const [format, setFormat] = useState<AuditFileFormat>(AuditFileFormat.Csv);
  const [busy, setBusy] = useState(false);
  const download = async () => {
    setBusy(true);
    try {
      const link = document.createElement('a');
      link.href = await services.audit.exportUrl(query, format);
      link.download = '';
      link.click();
    } catch (e) {
      actions.toast.show((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <span className={styles.export}>
      <Select value={format} aria-label="Export format" onChange={(e) => setFormat(e.target.value as AuditFileFormat)}>
        {Object.values(AuditFileFormat).map((f) => <option key={f} value={f}>{FormatLabel[f]}</option>)}
      </Select>
      <Button onClick={() => void download()} disabled={busy}>
        Export
      </Button>
    </span>
  );
}
