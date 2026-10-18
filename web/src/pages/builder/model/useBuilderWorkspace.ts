import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { SampleField } from '../../../api';
import type { RouteDraft } from '../../../domain';
import { BuilderCommands } from './BuilderCommands';
import type { BuilderStepContext } from './BuilderStepContext';
import { DestinationExtrasRules, type DestinationExtras } from './DestinationExtras';
import { DraftEditor, DraftSession } from './DraftSession';
import { FilterFieldCatalog } from './FilterFieldCatalog';
import { useBuilderConnections } from './useBuilderConnections';
import { useBuilderNavigation } from './useBuilderNavigation';
import { useBuilderPreview } from './useBuilderPreview';

/** Editing session of the builder: draft state, live preview, URL navigation and commands. */
export function useBuilderWorkspace(initialDraft: RouteDraft, fields: SampleField[]) {
  const [session, setSession] = useState(() => DraftSession.initial(initialDraft));
  const editor = useMemo(() => new DraftEditor(setSession), []);
  const navigate = useNavigate();
  const commands = useMemo(() => new BuilderCommands(navigate, editor), [navigate, editor]);
  const nav = useBuilderNavigation();
  const { draft, dirty } = session;
  const connections = useBuilderConnections(draft);
  const live = useBuilderPreview(draft, nav.sessionId, connections, fields);
  const { samples, pipeline: preview } = live;
  const catalog = useMemo(() => new FilterFieldCatalog(live.fields, samples, live.state.schema), [live.fields, samples, live.state.schema]);
  const [whyIndex, setWhyIndex] = useState<number | null>(null);
  const [deployOpen, setDeployOpen] = useState(false);
  const [extras, setExtrasState] = useState<DestinationExtras>(DestinationExtrasRules.initial);
  const ctx: BuilderStepContext = {
    draft,
    dirty,
    editor,
    step: nav.step,
    goTo: nav.goTo,
    samples,
    live,
    preview,
    catalog,
    connections,
    extras,
    setExtras: (patch, change) => {
      setExtrasState((prev) => ({ ...prev, ...patch }));
      editor.patchDestination({}, change);
    },
    whyIndex,
    setWhyIndex,
    selectedSampleId: nav.sampleId,
    selectSample: nav.selectSample,
    keepOnly: (field, value) => commands.keepOnly(catalog, field, value, nav.goTo),
    save: () => void commands.save(draft),
    runTest: () => void commands.test(live),
    openDeploy: () => setDeployOpen(true),
  };
  return { ctx, commands, deployOpen, closeDeploy: () => setDeployOpen(false) };
}
