import { Button, ButtonSize, ButtonVariant, ControlSize, Select, SegmentedControl, SegmentSize, TextArea, TextInput } from '../../../../../../components/ui';
import {
  AnswerPick,
  LookupErrorAction,
  LookupErrorActionLabel,
  LookupMissAction,
  LookupMissActionLabel,
  LookupMockMode,
  MockAnswer,
  type LookupMapRow,
  type LookupSourceStep,
} from '../../../../../../domain';
import styles from '../../../library/Library.module.css';

type EditorProps = { step: LookupSourceStep; onChange(next: LookupSourceStep): void };

/** Texts of the answer, policy and mock editors. */
const AnswerText = {
  map: 'Answer → fields',
  from: 'Pick from the answer',
  to: 'Write to field',
  onMiss: 'When there is no answer',
  onError: 'When the source fails (after retries)',
  defaultValue: 'Default value (JSON or text)',
  timeout: 'Timeout (ms)',
  retries: 'Retries',
  cache: 'Cache answers on each worker',
  ttl: 'TTL (ms)',
  negativeTtl: 'Miss TTL (ms)',
  maxEntries: 'Max entries',
  mock: 'Preview answer',
  mockJson: 'Mock answer (JSON)',
  mockInvalid: 'The mock answer is not valid JSON; the step is skipped until it parses.',
} as const;

const MockModes = [
  { value: LookupMockMode.Off, label: 'Live source' },
  { value: LookupMockMode.Hit, label: 'Mock answer' },
  { value: LookupMockMode.Miss, label: 'Mock miss' },
];

/** Answer picks (`$.tier`) → target fields, with add / remove. */
export function LookupMapEditor({ step, onChange }: EditorProps) {
  const write = (map: LookupMapRow[]) => onChange({ ...step, map });
  return (
    <div className={styles.settings}>
      <span className={styles.muted}>{AnswerText.map}</span>
      {step.map.map((row, i) => (
        <div key={i} className={styles.row}>
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={AnswerPick.root + '.tier'} aria-label={AnswerText.from} value={row.from}
            onChange={(e) => write(step.map.map((r, j) => (j === i ? { ...r, from: e.target.value } : r)))} />
          <span className={styles.muted}>→</span>
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder="customer.tier" aria-label={AnswerText.to} value={row.to}
            onChange={(e) => write(step.map.map((r, j) => (j === i ? { ...r, to: e.target.value } : r)))} />
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title="Remove" disabled={step.map.length === 1} onClick={() => write(step.map.filter((_, j) => j !== i))}>×</Button>
        </div>
      ))}
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => write([...step.map, { from: AnswerPick.root, to: '' }])}>+ Mapping</Button>
    </div>
  );
}

/** On-miss / on-error policies (with their default value), timeout, retries and the per-worker cache. */
export function LookupPolicyEditor({ step, onChange }: EditorProps) {
  const num = (text: string) => Math.max(0, Number(text) || 0);
  return (
    <div className={styles.settings}>
      <div className={styles.row}>
        <Select controlSize={ControlSize.Sm} sunken className={styles.grow} aria-label={AnswerText.onMiss} title={AnswerText.onMiss} value={step.onMiss.action}
          onChange={(e) => onChange({ ...step, onMiss: { ...step.onMiss, action: e.target.value as LookupMissAction } })}>
          {Object.values(LookupMissAction).map((a) => <option key={a} value={a}>{AnswerText.onMiss}: {LookupMissActionLabel[a]}</option>)}
        </Select>
        {step.onMiss.action === LookupMissAction.Default && (
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={AnswerText.defaultValue} aria-label={AnswerText.defaultValue} value={step.onMiss.value}
            onChange={(e) => onChange({ ...step, onMiss: { ...step.onMiss, value: e.target.value } })} />
        )}
      </div>
      <div className={styles.row}>
        <Select controlSize={ControlSize.Sm} sunken className={styles.grow} aria-label={AnswerText.onError} title={AnswerText.onError} value={step.onError.action}
          onChange={(e) => onChange({ ...step, onError: { ...step.onError, action: e.target.value as LookupErrorAction } })}>
          {Object.values(LookupErrorAction).map((a) => <option key={a} value={a}>{AnswerText.onError}: {LookupErrorActionLabel[a]}</option>)}
        </Select>
        {step.onError.action === LookupErrorAction.Default && (
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={AnswerText.defaultValue} aria-label={AnswerText.defaultValue} value={step.onError.value}
            onChange={(e) => onChange({ ...step, onError: { ...step.onError, value: e.target.value } })} />
        )}
      </div>
      <div className={styles.row}>
        <span className={styles.muted}>{AnswerText.timeout}</span>
        <TextInput controlSize={ControlSize.Sm} mono sunken type="number" className={styles.grow} aria-label={AnswerText.timeout} value={step.timeoutMs} onChange={(e) => onChange({ ...step, timeoutMs: num(e.target.value) })} />
        <span className={styles.muted}>{AnswerText.retries}</span>
        <TextInput controlSize={ControlSize.Sm} mono sunken type="number" className={styles.grow} aria-label={AnswerText.retries} value={step.retries} onChange={(e) => onChange({ ...step, retries: num(e.target.value) })} />
      </div>
      <LookupCacheEditor step={step} onChange={onChange} />
    </div>
  );
}

/** Cache toggle and its TTLs / size. */
function LookupCacheEditor({ step, onChange }: EditorProps) {
  const c = step.cache;
  const set = (patch: Partial<LookupSourceStep['cache']>) => onChange({ ...step, cache: { ...c, ...patch } });
  return (
    <>
      <label className={styles.row}>
        <input type="checkbox" checked={c.enabled} onChange={(e) => set({ enabled: e.target.checked })} />
        <span className={styles.muted}>{AnswerText.cache}</span>
      </label>
      {c.enabled && (
        <div className={styles.row}>
          {(['ttlMs', 'negativeTtlMs', 'maxEntries'] as const).map((key) => (
            <TextInput key={key} controlSize={ControlSize.Sm} mono sunken type="number" className={styles.grow} title={AnswerText[CacheLabels[key]]} aria-label={AnswerText[CacheLabels[key]]}
              value={c[key]} onChange={(e) => set({ [key]: Math.max(0, Number(e.target.value) || 0) })} />
          ))}
        </div>
      )}
    </>
  );
}

/** Label key of each cache number field. */
const CacheLabels = { ttlMs: 'ttl', negativeTtlMs: 'negativeTtl', maxEntries: 'maxEntries' } as const;

/** Studio-only preview answer: live source, a typed mock answer, or a mocked miss (sent as `lookupMocks`). */
export function LookupMockEditor({ step, onChange }: EditorProps) {
  const invalid = step.mockMode === LookupMockMode.Hit && step.mock.trim() !== '' && MockAnswer.parse(step.mock) === undefined;
  return (
    <div className={styles.settings}>
      <SegmentedControl size={SegmentSize.Xs} options={MockModes} value={step.mockMode} ariaLabel={AnswerText.mock} onChange={(mockMode) => onChange({ ...step, mockMode })} />
      {step.mockMode === LookupMockMode.Hit && (
        <TextArea controlSize={ControlSize.Sm} mono rows={4} aria-label={AnswerText.mockJson} placeholder='{"tier": "gold"}' value={step.mock} onChange={(e) => onChange({ ...step, mock: e.target.value })} />
      )}
      {invalid && <div className={styles.issues} role="alert">{AnswerText.mockInvalid}</div>}
    </div>
  );
}
