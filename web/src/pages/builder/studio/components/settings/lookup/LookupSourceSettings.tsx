import { ControlSize, Select } from '../../../../../../components/ui';
import { LookupSourceKindMeta, LookupStepDefaults, LookupStepRules, type LookupSource, type LookupSourceStep } from '../../../../../../domain';
import { useLookupSourceCatalog } from '../../../hooks/useLookupSourceCatalog';
import { LookupAnswerEditors } from './LookupAnswerEditorsGroup';
import { LookupRequestEditor } from './LookupRequestEditor';
import styles from '../../../library/Library.module.css';

/** Texts of the source picker. */
const SourceText = {
  pick: 'Lookup source',
  choose: 'Choose a lookup source…',
  none: 'No lookup sources yet — add one in Connections → Lookup sources.',
  missing: (id: string) => `${id} (not found)`,
} as const;

export type LookupSourceSettingsProps = { step: LookupSourceStep | undefined; onChange(next: LookupSourceStep): void };

/** Source mode of the lookup recipe: source picker, request template, answer mapping, policies, cache and preview mock. */
export function LookupSourceSettings({ step: current, onChange }: LookupSourceSettingsProps) {
  const sources = useLookupSourceCatalog();
  const step = current ?? LookupStepDefaults.step();
  const issue = LookupStepRules.issue(step);
  const pick = (id: string) => {
    const source = sources.find((s) => s.id === id);
    if (source) onChange(LookupStepDefaults.withSource(step, source.id, source.kind));
  };
  return (
    <div className={styles.settings}>
      <SourcePicker sources={sources} value={step.sourceId} onPick={pick} />
      {step.sourceId && <LookupRequestEditor request={step.request} onChange={(request) => onChange({ ...step, request })} />}
      {step.sourceId && <LookupAnswerEditors step={step} onChange={onChange} />}
      {issue && <span className={styles.muted}>{issue}</span>}
    </div>
  );
}

/** Select of a lookup source (name · kind · env); an id no longer in the list stays selectable. */
function SourcePicker({ sources, value, onPick }: { sources: LookupSource[]; value: string; onPick(id: string): void }) {
  const known = sources.some((s) => s.id === value);
  return (
    <Select controlSize={ControlSize.Sm} mono sunken value={value} aria-label={SourceText.pick} title={SourceText.pick} onChange={(e) => onPick(e.target.value)}>
      <option value="">{sources.length ? SourceText.choose : SourceText.none}</option>
      {value && !known && <option value={value}>{SourceText.missing(value)}</option>}
      {sources.map((s) => (
        <option key={s.id} value={s.id}>
          {s.name} · {LookupSourceKindMeta[s.kind].label} · {s.envTag}
        </option>
      ))}
    </Select>
  );
}
