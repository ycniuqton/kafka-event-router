import { useState } from 'react';
import { Button, ButtonSize, ButtonVariant } from '../../../../../components/ui';
import { SandboxSummary } from '../../../../../domain';
import { actions } from '../../../../../store';
import { useStudio } from '../../hooks/StudioContext';
import { useSandboxInfo } from '../../hooks/useSandboxInfo';
import { SampleRunReport } from '../../model/SampleRunReport';
import { SafetyRulesPanel } from './SafetyRulesPanel';
import styles from './JsStepSettings.module.css';

/** Bottom row of a JS step: Run on samples, Explain and the sandbox safety-rules toggle. */
export function JsStepActions({ onExplain }: { onExplain(): void }) {
  const { model } = useStudio();
  const [safetyOpen, setSafetyOpen] = useState(false);
  const [running, setRunning] = useState(false);
  const { info, error, reload } = useSandboxInfo();
  const runOnSamples = async () => {
    setRunning(true);
    const outcome = await model.preview.runNow();
    setRunning(false);
    if (outcome && !outcome.failure) actions.toast.show(new SampleRunReport(model.samples, outcome.result).ranMessage);
  };
  return (
    <>
      <div className={styles.actions}>
        <Button size={ButtonSize.Sm} onClick={() => void runOnSamples()} disabled={running}>
          Run on samples
        </Button>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={onExplain}>
          Explain
        </Button>
        <button type="button" className={styles.safety} aria-expanded={safetyOpen} onClick={() => setSafetyOpen(!safetyOpen)}>
          ⓘ {info ? SandboxSummary.badge(info) : SandboxSummary.sandboxed}
        </button>
      </div>
      {safetyOpen && <SafetyRulesPanel info={info} error={error} onRetry={reload} />}
    </>
  );
}
