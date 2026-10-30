import { Field, SegmentedControl, TextInput } from '../../../components/ui';
import { LookupSourceKind, LookupSourceKindMeta, type LookupLimits } from '../../../domain';
import { LookupKindFields } from './LookupKindFieldRegistry';
import { LookupDraftPatch, type LookupFormStepProps } from './model/LookupSourceForm';
import { NumberText } from './model/NumberText';
import styles from './LookupSourceForm.module.css';

/** Step 2 — the source kind (fixed once saved) and its settings. */
export function LookupKindStep(props: LookupFormStepProps) {
  const { draft, editing, update } = props;
  const options = Object.values(LookupSourceKind).map((k) => ({ value: k, label: LookupSourceKindMeta[k].label }));
  const Fields = LookupKindFields[draft.kind];
  return (
    <>
      {!editing && (
        <div className={styles.group}>
          <span className={styles.label}>Kind</span>
          <SegmentedControl size="md" options={options} value={draft.kind} onChange={(kind) => update(LookupDraftPatch.set(draft, 'kind', kind))} ariaLabel="Source kind" />
        </div>
      )}
      <Fields {...props} />
    </>
  );
}

/** Number fields of the Limits step, in display order. */
const LimitFields: { key: keyof LookupLimits; label: string; hint: string }[] = [
  { key: 'maxConcurrency', label: 'Max concurrent requests', hint: 'Per worker; extra calls wait for a free slot until their timeout.' },
  { key: 'ratePerSec', label: 'Rate limit (requests / s)', hint: '0 = no rate limit.' },
  { key: 'burst', label: 'Burst', hint: 'Requests allowed above the rate for short peaks.' },
  { key: 'breakerFailures', label: 'Failures before the circuit opens', hint: 'Consecutive transient failures.' },
  { key: 'breakerOpenMs', label: 'Circuit open time (ms)', hint: 'Calls fail fast as transient while open.' },
];

/** Step 3 — concurrency, rate limit and circuit breaker per source (shared by every route on a worker). */
export function LookupLimitsStep({ draft, errors, update }: LookupFormStepProps) {
  return (
    <>
      {LimitFields.map((f) => (
        <Field key={f.key} label={f.label} hint={f.hint} error={errors[`/limits/${f.key}`]}>
          <TextInput controlSize="lg" mono sunken type="number" value={NumberText.show(draft.limits[f.key])}
            onChange={(e) => update(LookupDraftPatch.limits(draft, { [f.key]: NumberText.of(e.target.value) }))} />
        </Field>
      ))}
    </>
  );
}
