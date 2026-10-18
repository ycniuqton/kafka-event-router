import { useState } from 'react';
import { Button, ButtonSize, ButtonVariant, ControlSize, TextInput } from '../../../../components/ui';
import { TestOutcomeLabel, TestVerdict, type TestCase, type TestCaseResult } from '../../../../domain';
import { actions } from '../../../../store';
import { useStudio } from '../hooks/StudioContext';
import { TestCaseBadge } from './TestCaseBadge';
import { TestCaseController, type TestCaseState } from './TestCaseController';
import { TestCaseDiff } from './TestCaseDiff';
import { TestCaseText } from './TestCaseText';
import { useTestCases } from './useTestCases';
import styles from './TestCasesPanel.module.css';

/** Saved test cases of the route under the steps: pin the current event, pass/fail badges, diffs (spec §8.5). */
export function TestCasesPanel() {
  const { model } = useStudio();
  const { state, controller } = useTestCases(model.routeId, model.steps, model.preview.run);
  if (!model.routeId || !controller) {
    return (
      <section className={styles.panel} aria-label={TestCaseText.title}>
        <div className={styles.head}><span className={styles.title}>{TestCaseText.title}</span></div>
        <div className={styles.hint}>{TestCaseText.unsaved}</div>
      </section>
    );
  }
  return (
    <section className={styles.panel} aria-label={TestCaseText.title} data-testid="test-cases">
      <PanelHead state={state} controller={controller} />
      {state.error && <div className={styles.error} role="alert">{state.error}</div>}
      {!state.loading && !state.cases.length && <div className={styles.hint}>{TestCaseText.empty}</div>}
      {state.cases.map((c) => (
        <TestCaseRow key={c.id} testCase={c} result={state.results.get(c.id)} controller={controller} />
      ))}
    </section>
  );
}

/** Title, pass/fail summary, Run and Pin. */
function PanelHead({ state, controller }: { state: TestCaseState; controller: TestCaseController }) {
  const { model } = useStudio();
  const results = [...state.results.values()];
  const passed = results.filter((r) => r.result === TestVerdict.Pass).length;
  const sample = model.current;
  const run = model.preview.run?.run(sample?.id);
  const pin = async () => {
    if (!sample || !run) return;
    const created = await controller.pin(TestCaseController.pinOf(sample, run, model.cursor.eventId));
    if (created) actions.toast.show(TestCaseText.pinned(created.name));
  };
  return (
    <div className={styles.head}>
      <span className={styles.title}>{TestCaseText.title}</span>
      {results.length > 0 && <span className={styles.summary} data-failing={results.length > passed}>{TestCaseText.summary(passed, results.length - passed)}</span>}
      <span className={styles.spacer} />
      {state.cases.length > 0 && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} disabled={state.running} onClick={() => void controller.rerun(model.steps)}>
          {state.running ? TestCaseText.running : TestCaseText.run}
        </Button>
      )}
      {!model.readOnly && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Secondary} disabled={!run} title={run ? TestCaseText.pinTitle : TestCaseText.noRun} onClick={() => void pin()}>
          {TestCaseText.pin}
        </Button>
      )}
    </div>
  );
}

type RowProps = { testCase: TestCase; result: TestCaseResult | undefined; controller: TestCaseController };

/** One case: badge, editable name, expected outcome, and for failures the diff and re-lock actions. */
function TestCaseRow({ testCase, result, controller }: RowProps) {
  const { model } = useStudio();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(testCase.name);
  const failing = !!result && result.result !== TestVerdict.Pass;
  return (
    <div className={styles.row} data-failing={failing} data-testid="test-case">
      <div className={styles.line}>
        <TestCaseBadge result={result} />
        <TextInput controlSize={ControlSize.Sm} sunken className={styles.name} value={name} aria-label={TestCaseText.rename} disabled={model.readOnly}
          onChange={(e) => setName(e.target.value)} onBlur={() => name.trim() && name !== testCase.name && void controller.rename(testCase, name.trim())} />
        <span className={styles.expects}>{TestCaseText.expects} {TestOutcomeLabel[testCase.expectedOutcome]}</span>
        {failing && <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setOpen(!open)}>{open ? TestCaseText.hideDiff : TestCaseText.diff}</Button>}
        {!model.readOnly && (
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={TestCaseText.delete} aria-label={TestCaseText.delete} onClick={() => void controller.remove(testCase)}>✕</Button>
        )}
      </div>
      {failing && result?.error && <div className={styles.message}>{result.error}</div>}
      {failing && open && result && <TestCaseDiff testCase={testCase} result={result} controller={controller} />}
    </div>
  );
}
