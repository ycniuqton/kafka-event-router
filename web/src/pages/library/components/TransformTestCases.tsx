import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, ControlSize, EmptyState, Field, TextArea, ToneBadge } from '../../../components/ui';
import { EventStatus, TestVerdictLabel, TestVerdictTone, type InlineEvent, type JsonValue, type TestCase, type TestCaseResult } from '../../../domain';
import { useServiceQuery } from '../../../store';
import styles from './LibraryList.module.css';

/** Texts of a transform's test cases. */
class TransformCaseText {
  static readonly run = 'Run all';
  static readonly add = 'Add case';
  static readonly input = 'Input event value (JSON object)';
  static readonly invalid = 'Paste a JSON object.';
  static readonly empty = 'No test cases yet. Add one to lock what this transform produces.';
  static readonly remove = 'Delete';
  static readonly summary = (passed: number, failed: number) => `${passed} passed · ${failed} failed`;
  static readonly name = (n: number) => `Case ${n}`;
}

/** Test cases of a library transform: add (locks the latest version's actual result), run all, delete. */
export function TransformTestCases({ transformId }: { transformId: string }) {
  const [refresh, setRefresh] = useState(0);
  const { data: cases, error } = useServiceQuery(() => services.library.testCases(transformId), [transformId, refresh]);
  const [results, setResults] = useState<Map<string, TestCaseResult>>(new Map());
  const [summary, setSummary] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const runAll = async () => {
    try {
      const report = await services.library.runTestCases(transformId);
      setResults(new Map(report.results.map((r) => [r.caseId, r])));
      setSummary(TransformCaseText.summary(report.passed, report.failed));
    } catch (e) {
      setFailure((e as Error).message);
    }
  };
  const remove = async (c: TestCase) => {
    await services.library.removeTestCase(c);
    setRefresh(refresh + 1);
  };
  return (
    <div className={styles.detail}>
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} disabled={!cases?.length} onClick={() => void runAll()}>{TransformCaseText.run}</Button>
        {summary && <span className={styles.secondary}>{summary}</span>}
      </div>
      {(error || failure) && <div className={styles.error} role="alert">{error?.message ?? failure}</div>}
      {cases && !cases.length && <EmptyState compact title={TransformCaseText.empty} />}
      <div className={styles.list} aria-label="Test cases">
        {(cases ?? []).map((c) => {
          const r = results.get(c.id);
          return (
            <div key={c.id} className={[styles.item, styles.itemHead].join(' ')}>
              <span className={styles.grow}>{c.name}</span>
              {r && <ToneBadge tone={TestVerdictTone[r.result]}>{TestVerdictLabel[r.result]}</ToneBadge>}
              {r?.error && <span className={styles.muted}>{r.error}</span>}
              <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void remove(c)}>{TransformCaseText.remove}</Button>
            </div>
          );
        })}
      </div>
      <AddCase transformId={transformId} next={(cases?.length ?? 0) + 1} onAdded={() => setRefresh(refresh + 1)} />
    </div>
  );
}

/** Pins an input: creates the case, runs it on the latest version and locks the actual outcome and output. */
function AddCase({ transformId, next, onAdded }: { transformId: string; next: number; onAdded(): void }) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const add = async () => {
    let value: JsonValue;
    try {
      value = JSON.parse(text) as JsonValue;
    } catch {
      return setError(TransformCaseText.invalid);
    }
    try {
      const input: InlineEvent = { key: null, value, headers: [] };
      const created = await services.library.addTestCase(transformId, { name: TransformCaseText.name(next), inputEvent: input, expectedOutcome: EventStatus.Error, expectedOutput: null });
      const run = await services.library.runTestCases(transformId, undefined, [created.id]);
      const actual = run.results[0];
      if (actual) await services.library.updateTestCase(created, { expectedOutcome: actual.actualOutcome, expectedOutput: actual.actualOutput });
      setText('');
      setError(null);
      onAdded();
    } catch (e) {
      setError((e as Error).message);
    }
  };
  return (
    <Field label={TransformCaseText.input} error={error ?? undefined}>
      <TextArea controlSize={ControlSize.Sm} mono rows={4} value={text} onChange={(e) => setText(e.target.value)} aria-label={TransformCaseText.input} />
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} disabled={!text.trim()} onClick={() => void add()}>{TransformCaseText.add}</Button>
      </div>
    </Field>
  );
}
