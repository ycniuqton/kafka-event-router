import { useCallback, useMemo, useState } from 'react';
import { useAccess } from '../../../../access';
import { AccessPolicy, JsonFlattener, Permission } from '../../../../domain';
import type { TransformStudioProps } from '../contract';
import { SampleCursor } from '../model/SampleCursor';
import { SampleRunReport } from '../model/SampleRunReport';
import { StepBaseline } from '../model/StepBaseline';
import type { StepEdit } from '../model/StepListEditor';
import { InputView, SidePane } from '../model/StudioEnums';
import type { StudioCommands, StudioContextValue, StudioModel, StudioPreview, StudioUiState } from './StudioContext';
import type { StudioSampleSource } from '../preview/StudioRunPlan';
import { useStudioPreview } from './useStudioPreview';

/** Initial UI state: diff on, output after all steps, tree view, Output tab on narrow screens. */
class StudioUiDefaults {
  static readonly state: StudioUiState = {
    selectedLine: null,
    selectedStepId: null,
    diff: true,
    atStep: false,
    inputView: InputView.Tree,
    sidePane: SidePane.Output,
    exportSelection: null,
    byExample: false,
    explainChain: false,
  };
}

/** Derives the studio model from the builder's props and owns the UI-only state. */
export function useStudioState(props: TransformStudioProps): StudioContextValue {
  const { steps, samples, selectedSampleId, onStepsChange, onSelectSample } = props;
  const [ui, setUiState] = useState<StudioUiState>(StudioUiDefaults.state);
  const readOnly = useReadOnly();
  const baseline = useStepBaseline(steps, props.stepsSaved ?? true);
  const source = useSampleSource(props.sampleSessionId ?? null, props.connectionId ?? null);
  const model = useStudioModel(steps, samples, selectedSampleId, ui, readOnly, baseline, props.routeId ?? null, source);
  const setUi = useCallback((patch: Partial<StudioUiState>) => setUiState((prev) => ({ ...prev, ...patch })), []);
  const commands = useMemo<StudioCommands>(
    () => ({
      commit: (edit: StepEdit | null) => {
        if (edit && !readOnly) onStepsChange(edit.steps, edit.label);
      },
      selectSample: (id) => {
        setUi({ selectedLine: null });
        onSelectSample(id);
      },
      selectStep: (id) => setUi({ selectedStepId: id }),
      selectLine: (path) => setUi({ selectedLine: path }),
      setUi,
    }),
    [onStepsChange, onSelectSample, setUi, readOnly],
  );
  return useMemo(() => ({ model, ui, commands }), [model, ui, commands]);
}

/** Where the samples come from (stable while the session and connection stay the same). */
function useSampleSource(sessionId: string | null, connectionId: string | null): StudioSampleSource {
  return useMemo(() => ({ sessionId, connectionId }), [sessionId, connectionId]);
}

/** Baseline of the saved steps: follows the steps while they are saved, frozen while there are edits. */
function useStepBaseline(steps: TransformStudioProps['steps'], saved: boolean): StepBaseline {
  const [savedSteps, setSavedSteps] = useState(steps);
  if (saved && savedSteps !== steps) setSavedSteps(steps);
  return useMemo(() => new StepBaseline(savedSteps), [savedSteps]);
}

/** Read-only once access is known and lacks transform.write (no flicker while access loads). */
function useReadOnly(): boolean {
  const access = useAccess();
  return access !== null && !AccessPolicy.can(access, Permission.TransformWrite);
}

function useStudioModel(
  steps: TransformStudioProps['steps'],
  samples: TransformStudioProps['samples'],
  selectedSampleId: number | null,
  ui: StudioUiState,
  readOnly: boolean,
  baseline: StepBaseline,
  routeId: string | null,
  source: StudioSampleSource,
): StudioModel {
  const connectionId = source.connectionId;
  const cursor = useMemo(() => new SampleCursor(samples, selectedSampleId), [samples, selectedSampleId]);
  const live = useStudioPreview(steps, samples, source);
  const { result: run, pending, failure, runNow } = live;
  const preview = useMemo<StudioPreview>(() => ({ run, pending, failure, runNow }), [run, pending, failure, runNow]);
  const report = useMemo(() => new SampleRunReport(samples, run), [samples, run]);
  const current = cursor.current;
  const inputLines = useMemo(() => (current ? JsonFlattener.flatten(current.value) : []), [current]);
  const selectedStepIndex = steps.findIndex((s) => s.id === ui.selectedStepId);
  const upto = ui.atStep && selectedStepIndex >= 0 ? selectedStepIndex : undefined;
  const result = useMemo(() => (current && run ? run.resultFor(current.id, upto) : null), [current, run, upto]);
  return useMemo(
    () => ({ steps, samples, cursor, current, inputLines, result, report, preview, selectedStepIndex, readOnly, baseline, routeId, connectionId }),
    [steps, samples, cursor, current, inputLines, result, report, preview, selectedStepIndex, readOnly, baseline, routeId, connectionId],
  );
}
