import { useState, type CSSProperties } from 'react';
import { Button, ButtonSize, ButtonVariant, ControlSize, TextArea, TextInput } from '../../../components/ui';
import { LibraryRules, type ImportIssue } from '../../../domain';
import { LibraryLayout } from '../model/LibraryLayout';
import { LookupGridModel } from '../model/LookupGridModel';
import styles from './LibraryList.module.css';

/** Texts of the grid editor. */
class GridText {
  static readonly key = 'Key';
  static readonly value = 'Value';
  static readonly addRow = 'Add row';
  static readonly removeRow = 'Remove row';
  static readonly paste = 'Paste CSV (key,value) to add or replace rows';
  static readonly applyPaste = 'Apply paste';
}

type LookupGridEditorProps = { grid: LookupGridModel; onChange(grid: LookupGridModel): void };

/** Editable key → value grid: edit cells, add/remove rows, paste CSV; empty and duplicate keys are listed. */
export function LookupGridEditor({ grid, onChange }: LookupGridEditorProps) {
  const [paste, setPaste] = useState('');
  const [pasteIssues, setPasteIssues] = useState<ImportIssue[]>([]);
  const issues = grid.issues();
  const applyPaste = () => {
    const result = grid.paste(paste);
    setPasteIssues(result.issues);
    if (!result.issues.length) {
      onChange(result.grid);
      setPaste('');
    }
  };
  return (
    <div className={styles.detail}>
      <div className={styles.grid} style={{ '--cols': LibraryLayout.rowColumns } as CSSProperties} aria-label="Rows editor">
        <span className={styles.gridHead}>{GridText.key}</span>
        <span className={styles.gridHead}>{GridText.value}</span>
        <span />
        {grid.rows.map((r, i) => [
          <TextInput key={`${i}-k`} controlSize={ControlSize.Sm} mono sunken value={r.key} aria-label={`${GridText.key} ${i + 1}`} onChange={(e) => onChange(grid.set(i, { key: e.target.value }))} />,
          <TextInput key={`${i}-v`} controlSize={ControlSize.Sm} mono sunken value={r.value} aria-label={`${GridText.value} ${i + 1}`} onChange={(e) => onChange(grid.set(i, { value: e.target.value }))} />,
          <Button key={`${i}-x`} size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={GridText.removeRow} onClick={() => onChange(grid.remove(i))}>×</Button>,
        ])}
      </div>
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => onChange(grid.add())}>{GridText.addRow}</Button>
      </div>
      <IssueList issues={[...issues, ...pasteIssues]} />
      <TextArea controlSize={ControlSize.Sm} mono rows={3} placeholder={GridText.paste} aria-label={GridText.paste} value={paste} onChange={(e) => setPaste(e.target.value)} />
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} disabled={!paste.trim()} onClick={applyPaste}>{GridText.applyPaste}</Button>
      </div>
    </div>
  );
}

/** Per-line issues of a grid or an import. */
export function IssueList({ issues }: { issues: ImportIssue[] }) {
  if (!issues.length) return null;
  return (
    <div className={styles.issues} role="alert">
      {issues.map((i, n) => <span key={`${n}-${i.line}`}>{LibraryRules.issueLabel(i)}</span>)}
    </div>
  );
}
