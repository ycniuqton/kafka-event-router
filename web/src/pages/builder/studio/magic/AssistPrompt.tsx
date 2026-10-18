import { useState } from 'react';
import { Button, ButtonSize, ControlSize, TextInput } from '../../../../components/ui';
import { actions } from '../../../../store';
import { services } from '../../../../api';
import { useStudio } from '../hooks/StudioContext';
import { StepListEditor } from '../model/StepListEditor';
import { StepChangeLabel, StudioText } from '../model/StudioText';
import { AssistSamples } from './AssistSamples';
import { ProposalDiff } from './ProposalDiff';
import { ProposalSelection, ProposalText } from './ProposalSelection';
import styles from './Magic.module.css';

/** Texts of the AI prompt box. */
class AssistPromptText {
  static readonly placeholder = 'Describe it: hide the email, rename cust_id to customerId…';
  static readonly label = 'Describe the transform in words';
  static readonly propose = 'Propose';
  static readonly thinking = 'Thinking…';
}

/** "Describe it in words" (spec §8.6): proposes recipe steps shown as a diff; nothing is applied without Accept. */
export function AssistPrompt() {
  const { model, commands } = useStudio();
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selection, setSelection] = useState<ProposalSelection | null>(null);
  const propose = async () => {
    setBusy(true);
    setError(null);
    try {
      const fields = model.inputLines.map((line) => line.path);
      setSelection(new ProposalSelection(await services.assist.propose(text, { steps: model.steps, fields, ...AssistSamples.of(model) })));
    } catch (e) {
      setError((e as Error).message || StudioText.assistUnavailable);
    } finally {
      setBusy(false);
    }
  };
  const accept = () => {
    const steps = selection?.accepted() ?? [];
    if (!steps.length) return;
    commands.commit(new StepListEditor(model.steps).append(steps, StepChangeLabel.aiProposed));
    actions.toast.show(StudioText.addedSteps(steps.length));
    setSelection(null);
    setText('');
  };
  return (
    <div className={styles.box}>
      <form className={styles.row} onSubmit={(e) => {
          e.preventDefault();
          if (text.trim()) void propose();
        }}>
        <TextInput controlSize={ControlSize.Sm} sunken className={styles.input} value={text} onChange={(e) => setText(e.target.value)} placeholder={AssistPromptText.placeholder} aria-label={AssistPromptText.label} />
        <Button type="submit" size={ButtonSize.Sm} disabled={busy || !text.trim()}>
          {busy ? AssistPromptText.thinking : AssistPromptText.propose}
        </Button>
      </form>
      {error && <div className={styles.error} role="alert">{error}</div>}
      {selection && selection.proposals.length > 0 && <ProposalDiff selection={selection} onChange={setSelection} onAccept={accept} onReject={() => setSelection(null)} />}
      {selection && selection.proposals.length === 0 && <div className={styles.none}>{ProposalText.none}</div>}
    </div>
  );
}
