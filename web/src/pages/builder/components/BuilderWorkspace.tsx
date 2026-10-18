import type { SampleField } from '../../../api';
import { BuilderStep, type RouteDraft } from '../../../domain';
import { Keys, useShortcut } from '../../../store';
import { RailModel } from '../model/RailModel';
import { ReviewModel } from '../model/ReviewModel';
import { useBuilderWorkspace } from '../model/useBuilderWorkspace';
import { TestRunDialog } from '../../../components/sample-panel';
import { BuilderSamplePanel } from './BuilderSamplePanel';
import { BuilderTopBar } from './BuilderTopBar';
import { DeployModal } from './DeployModal';
import { PipelineRail } from './PipelineRail';
import { StepEditor } from './StepEditor';
import styles from './BuilderWorkspace.module.css';

export type BuilderWorkspaceProps = { initialDraft: RouteDraft; fields: SampleField[] };

/** The three-zone builder: pipeline rail, step editor and live samples, under the builder top bar. */
export function BuilderWorkspace({ initialDraft, fields }: BuilderWorkspaceProps) {
  const { ctx, commands, deployOpen, closeDeploy } = useBuilderWorkspace(initialDraft, fields);
  const { draft, step } = ctx;
  useShortcut({ key: Keys.S, mod: true, handler: ctx.save });
  useShortcut({ key: Keys.Enter, mod: true, handler: ctx.runTest });
  return (
    <div className={styles.root}>
      <BuilderTopBar
        name={draft.name}
        status={draft.status}
        dirty={ctx.dirty.size > 0}
        onRename={(name) => ctx.editor.rename(name)}
        onBack={() => commands.back(draft)}
        onSave={ctx.save}
        onTest={ctx.runTest}
        onDeploy={() => ctx.goTo(BuilderStep.Review)}
      />
      <div className={styles.zones}>
        <PipelineRail cards={RailModel.build(draft, ctx.preview, ctx.dirty, ctx.connections)} active={step} onSelect={ctx.goTo} draft={draft} editor={ctx.editor} />
        <section className={styles.centre}>
          <StepEditor ctx={ctx} commands={commands} />
        </section>
        {step !== BuilderStep.Transform && <BuilderSamplePanel ctx={ctx} />}
      </div>
      <TestRunDialog result={ctx.live.state.testResult} onClose={() => ctx.live.actions.clearTest()}
        onWriteToTopic={() => void ctx.live.actions.writeTestTopic(ctx.live.config, draft.routeId ?? undefined)} testPhase={ctx.live.state.testPhase} />
      <DeployModal open={deployOpen} title={ReviewModel.deployTitle(draft)} onClose={closeDeploy} onDeploy={(note, overwrite) => commands.deploy(draft, note, overwrite)} onReload={() => commands.reload()} />
    </div>
  );
}
