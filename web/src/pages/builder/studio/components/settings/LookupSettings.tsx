import { useState } from 'react';
import { ControlSize, SegmentedControl, SegmentSize, TextArea, TextInput } from '../../../../../components/ui';
import { LibraryKind, LibraryRules, LookupCsv, LookupMode, type ImportIssue } from '../../../../../domain';
import { LibraryPicker } from '../../library/LibraryPicker';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import { LookupSourceSettings } from './lookup/LookupSourceSettings';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from '../../library/Library.module.css';

/** Texts of the lookup recipe form. */
class LookupText {
  static readonly modes = [
    { value: LookupMode.Inline, label: 'Inline rows' },
    { value: LookupMode.Table, label: 'Library table' },
    { value: LookupMode.Source, label: 'External source' },
  ];
  static readonly mode = 'Lookup table source';
  static readonly target = 'Write to (empty = replace the key)';
  static readonly fallback = 'Default when the key is missing (empty = keep)';
  static readonly rows = 'Rows as CSV (key,value)';
  static readonly rowCount = (n: number) => `${n} row${n === 1 ? '' : 's'}`;
}

/** Lookup recipe: key field → value from inline rows (CSV text) or a pinned library table; or a call to a lookup source. */
export function LookupSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  const mode = step.lookupMode ?? LookupMode.Inline;
  const modes = <SegmentedControl size={SegmentSize.Xs} options={LookupText.modes} value={mode} ariaLabel={LookupText.mode} onChange={(lookupMode) => update({ lookupMode }, label)} />;
  if (mode === LookupMode.Source) {
    return (
      <div className={styles.settings}>
        {modes}
        <LookupSourceSettings step={step.lookupSource} onChange={(lookupSource) => update({ lookupSource }, label)} />
      </div>
    );
  }
  return (
    <div className={styles.settings}>
      <div className={styles.row}>
        <FieldSelect className={styles.grow} value={step.field} onChange={(field) => update({ field }, label)} />
        <span className={styles.muted}>→</span>
        <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder="region" value={step.argument}
          onChange={(e) => update({ argument: e.target.value }, label)} aria-label={LookupText.target} title={LookupText.target} />
      </div>
      {modes}
      {mode === LookupMode.Table ? (
        <LibraryPicker kind={LibraryKind.LookupTable} name={step.lookupRef ?? ''} version={step.lookupVersion}
          onChange={(lookupRef, lookupVersion) => update({ lookupRef, lookupVersion }, label)} />
      ) : (
        <InlineRows step={step} update={update} index={0} />
      )}
      <TextInput controlSize={ControlSize.Sm} mono sunken placeholder={LookupText.fallback} value={step.fallback ?? ''}
        onChange={(e) => update({ fallback: e.target.value }, label)} aria-label={LookupText.fallback} />
    </div>
  );
}

/** Inline rows edited as CSV text; rows update when the text parses, per-line issues otherwise. */
function InlineRows({ step, update }: StepSettingsProps) {
  const [text, setText] = useState(() => LookupCsv.format(step.lookupRows ?? []));
  const [issues, setIssues] = useState<ImportIssue[]>([]);
  const change = (next: string) => {
    setText(next);
    const parsed = LookupCsv.parse(next);
    setIssues(parsed.issues);
    if (!parsed.issues.length) update({ lookupRows: parsed.rows }, StepChangeLabel.edited(step.kind));
  };
  return (
    <>
      <TextArea controlSize={ControlSize.Sm} mono rows={5} value={text} onChange={(e) => change(e.target.value)} aria-label={LookupText.rows} />
      <span className={styles.muted}>{LookupText.rowCount(step.lookupRows?.length ?? 0)}</span>
      {issues.length > 0 && (
        <div className={styles.issues} role="alert">
          {issues.map((i) => (
            <span key={i.line}>{LibraryRules.issueLabel(i)}</span>
          ))}
        </div>
      )}
    </>
  );
}
