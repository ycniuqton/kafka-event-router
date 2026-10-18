import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { useStudio } from '../hooks/StudioContext';
import { ExplainPanel } from './ExplainPanel';
import { ExplainText } from './MagicText';

/** "Explain steps" toggle in the steps header (rule-based summary of the enabled recipe chain). */
export function ExplainChainButton() {
  const { model, ui, commands } = useStudio();
  if (!model.steps.some((s) => s.enabled)) return null;
  return (
    <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} aria-pressed={ui.explainChain} onClick={() => commands.setUi({ explainChain: !ui.explainChain })}>
      {ui.explainChain ? ExplainText.hide : ExplainText.explainChain}
    </Button>
  );
}

/** The chain explanation, shown above the step cards while toggled on (not persisted). */
export function ExplainChain() {
  const { model, ui } = useStudio();
  const enabled = model.steps.filter((s) => s.enabled);
  if (!ui.explainChain || !enabled.length) return null;
  return <ExplainPanel steps={enabled} />;
}
