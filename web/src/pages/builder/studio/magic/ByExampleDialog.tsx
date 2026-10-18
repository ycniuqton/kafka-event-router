import { useState } from 'react';
import { services, type InferredMapping } from '../../../../api';
import { Button, ButtonSize, ButtonVariant, ControlSize, Field, Modal, ModalWidth, TextArea } from '../../../../components/ui';
import type { JsonValue } from '../../../../domain';
import { actions } from '../../../../store';
import { useStudio } from '../hooks/StudioContext';
import { StepListEditor } from '../model/StepListEditor';
import { StudioText } from '../model/StudioText';
import { MappingTable } from './MappingTable';
import { ProposalDiff } from './ProposalDiff';
import { ProposalSelection, ProposalText } from './ProposalSelection';
import { ByExampleText } from './MagicText';
import styles from './Magic.module.css';


/** Paste a desired output → POST /preview/infer-mapping → per-target confidence → accept or reject proposed steps. */
export function ByExampleDialog() {
  const { ui } = useStudio();
  const flow = useByExample();
  if (!ui.byExample) return null;
  const footer = (
    <Button variant={ButtonVariant.Primary} disabled={flow.busy || !flow.text.trim()} onClick={() => void flow.infer()}>
      {flow.busy ? ByExampleText.working : ByExampleText.infer}
    </Button>
  );
  return (
    <Modal open onClose={flow.close} title={ByExampleText.title} subtitle={ByExampleText.subtitle} width={ModalWidth.Lg} footer={footer}>
      <div className={styles.example}>
        <Field label={ByExampleText.label} error={flow.error ?? undefined}>
          <TextArea controlSize={ControlSize.Sm} mono rows={10} value={flow.text} onChange={(e) => flow.setText(e.target.value)} aria-label={ByExampleText.label} />
        </Field>
        <div>
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} disabled={!flow.canSeed} onClick={flow.seed}>{ByExampleText.seed}</Button>
        </div>
        {flow.result && <MappingTable inferred={flow.result.inferred} />}
        {flow.result && flow.result.selection.proposals.length > 0 && (
          <ProposalDiff selection={flow.result.selection} onChange={flow.select} onAccept={flow.accept} onReject={flow.reset} />
        )}
      </div>
    </Modal>
  );
}

type ByExampleResult = { inferred: InferredMapping; selection: ProposalSelection };

/** State and actions of the by-example dialog: parse the pasted JSON, infer, accept the selected steps. */
function useByExample() {
  const { model, commands } = useStudio();
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ByExampleResult | null>(null);
  const close = () => {
    commands.setUi({ byExample: false });
    setResult(null);
    setError(null);
  };
  const infer = async () => {
    const desired = ExampleInput.parse(text);
    if (!model.current || desired === undefined) return setError(model.current ? ByExampleText.invalidJson : ByExampleText.noEvent);
    setBusy(true);
    setError(null);
    try {
      const inferred = await services.assist.inferMapping({ key: model.current.key, value: model.current.value }, desired);
      setResult({ inferred, selection: new ProposalSelection(inferred.proposals) });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const accept = () => {
    const steps = result?.selection.accepted() ?? [];
    commands.commit(new StepListEditor(model.steps).append(steps, ProposalText.addedByExample));
    actions.toast.show(StudioText.addedSteps(steps.length));
    setText('');
    close();
  };
  const seed = () => setText(model.current ? JSON.stringify(model.current.value, null, ByExampleText.indent) : '');
  const select = (selection: ProposalSelection) => setResult(result && { ...result, selection });
  return { text, setText, busy, error, result, close, infer, accept, seed, select, reset: () => setResult(null), canSeed: !!model.current };
}

/** Parses the pasted example (undefined when it is not JSON). */
class ExampleInput {
  static parse(text: string): JsonValue | undefined {
    try {
      return JSON.parse(text) as JsonValue;
    } catch {
      return undefined;
    }
  }
}
