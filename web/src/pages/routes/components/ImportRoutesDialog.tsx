import { Button, Modal, ModalWidth, SegmentedControl, SegmentSize, TextArea, ToneBadge } from '../../../components/ui';
import type { ImportReport } from '../../../domain';
import { ImportReportModel } from '../model/ImportReportModel';
import { useImportDialog } from '../model/useImportDialog';
import styles from './ImportRoutesDialog.module.css';

type ImportRoutesDialogProps = { open: boolean; onClose: () => void };

/** Import routes from YAML / JSON (file or paste): preview with a dry run, then import as drafts (never deploys). */
export function ImportRoutesDialog({ open, onClose }: ImportRoutesDialogProps) {
  const d = useImportDialog(onClose);
  const imported = d.report !== null && !d.report.dryRun;
  const empty = d.busy || !d.content.trim();
  const footer = (
    <>
      <Button size="lg" onClick={d.close}>{imported ? 'Done' : 'Cancel'}</Button>
      {!imported && <Button size="lg" onClick={() => void d.run(true)} disabled={empty}>Preview</Button>}
      {!imported && <Button size="lg" variant="primary" onClick={() => void d.run(false)} disabled={empty}>Import as drafts</Button>}
    </>
  );
  return (
    <Modal open={open} onClose={d.close} title="Import routes" subtitle="YAML or JSON exported from a router. Imports create or update drafts; nothing is deployed."
      width={ModalWidth.Lg} footer={footer}>
      <div className={styles.body}>
        <div className={styles.row}>
          <input type="file" accept=".yaml,.yml,.json,application/yaml,application/json" aria-label="Route file" onChange={(e) => void d.pickFile(e)} />
          {d.fileName && <span className={styles.file}>{d.fileName}</span>}
          <SegmentedControl options={ImportReportModel.modeOptions} value={d.mode} onChange={d.setMode} size={SegmentSize.Sm} ariaLabel="Existing names" />
        </div>
        <TextArea mono block className={styles.area} spellCheck={false} placeholder="Paste route YAML or JSON…" aria-label="Route documents" value={d.content}
          onChange={(e) => d.setContent(e.target.value)} />
        {d.error && <div className={styles.error}>{d.error}</div>}
        {d.report && <ImportResults report={d.report} />}
      </div>
    </Modal>
  );
}

/** One line per document: name, outcome and its issues. */
function ImportResults({ report }: { report: ImportReport }) {
  return (
    <>
      <div className={styles.summary}>{report.dryRun ? 'Preview: ' : 'Imported: '}{ImportReportModel.summary(report)}</div>
      <div className={styles.results}>
        {report.items.map((item) => (
          <div key={item.index} className={styles.item}>
            <div className={styles.head}>
              <span className={styles.name}>{item.name || `document ${item.index + 1}`}</span>
              <ToneBadge tone={ImportReportModel.tone(item)}>{ImportReportModel.label(item, report.dryRun)}</ToneBadge>
            </div>
            {item.issues.map((issue) => (
              <div key={`${issue.path}-${issue.code}`} className={styles.issue} data-tone={ImportReportModel.issueTone(issue.severity)}>
                {issue.path || '/'} · {issue.message}
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  );
}
