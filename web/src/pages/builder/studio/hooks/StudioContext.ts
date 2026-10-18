import { createContext, useContext } from 'react';
import type { JsonLine, PreviewFailure, SampleEvent, TransformRunResult, TransformStep } from '../../../../domain';
import type { StudioRunResult } from '../preview/StudioRunResult';
import type { StudioRunOutcome } from './useStudioPreview';
import type { SampleCursor } from '../model/SampleCursor';
import type { SampleRunReport } from '../model/SampleRunReport';
import type { StepBaseline } from '../model/StepBaseline';
import type { StepEdit } from '../model/StepListEditor';
import type { InputView, SidePane } from '../model/StudioEnums';

/** UI-only studio state (design builder fields studioSel, selLine, diff, atStep). */
export type StudioUiState = {
  selectedLine: string | null;
  selectedStepId: number | null;
  diff: boolean;
  atStep: boolean;
  inputView: InputView;
  sidePane: SidePane;
  /** Step ids picked for "Export to JS"; null while not selecting. */
  exportSelection: number[] | null;
  /** Transform-by-example dialog open (header button or the mapper's "Paste an example"). */
  byExample: boolean;
  /** Plain-language summary of the whole chain shown above the cards. */
  explainChain: boolean;
};

/** State of the preview service behind the Studio (last good run, in-flight run, last failure). */
export type StudioPreview = { run: StudioRunResult | null; pending: boolean; failure: PreviewFailure | null; runNow(): Promise<StudioRunOutcome> };

/** Values derived from the steps and the current sample. */
export type StudioModel = {
  steps: TransformStep[];
  samples: SampleEvent[];
  cursor: SampleCursor;
  current: SampleEvent | undefined;
  inputLines: JsonLine[];
  result: TransformRunResult | null;
  report: SampleRunReport;
  preview: StudioPreview;
  selectedStepIndex: number;
  /** Viewers see the studio without edit controls; edits are ignored. */
  readOnly: boolean;
  /** Steps as last saved (unsaved-change dots). */
  baseline: StepBaseline;
  /** Saved route id (test cases), null for a new route. */
  routeId: string | null;
  /** Source connection of the draft (masking of assistant samples), null when unknown. */
  connectionId: string | null;
};

/** Everything the studio's panes can do. */
export type StudioCommands = {
  commit(edit: StepEdit | null): void;
  selectSample(id: number | null): void;
  selectStep(id: number | null): void;
  selectLine(path: string | null): void;
  setUi(patch: Partial<StudioUiState>): void;
};

export type StudioContextValue = { model: StudioModel; ui: StudioUiState; commands: StudioCommands };

export const StudioContext = createContext<StudioContextValue | null>(null);

/** Studio state for components rendered inside <TransformStudio>. */
export function useStudio(): StudioContextValue {
  return useContext(StudioContext) as StudioContextValue;
}
