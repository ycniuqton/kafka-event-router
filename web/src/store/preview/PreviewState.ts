import {
  SampleView,
  type FieldSchema,
  type LoadSamplesRequest,
  type PreviewFailure,
  type RunResult,
  type SampleRecord,
  type SampleSession,
} from '../../domain';

/** Lifecycle of the sample load and of preview runs. */
export const PreviewPhase = { Idle: 'idle', Loading: 'loading', Ready: 'ready', Failed: 'failed' } as const;
export type PreviewPhase = (typeof PreviewPhase)[keyof typeof PreviewPhase];

/** State of one sample panel: session, loaded records, schema, the latest run and test run. */
export type PreviewState = {
  request: LoadSamplesRequest | null;
  session: SampleSession | null;
  records: SampleRecord[];
  schema: FieldSchema | null;
  loadPhase: PreviewPhase;
  loadFailure: PreviewFailure | null;
  result: RunResult | null;
  runPhase: PreviewPhase;
  runFailure: PreviewFailure | null;
  testResult: RunResult | null;
  testPhase: PreviewPhase;
  query: string;
  searchHits: ReadonlySet<number> | null;
  selectedSeq: number | null;
  view: SampleView;
  /** Branch the panel shows (null = first branch / event status). */
  branch: string | null;
  /** Format override in flight. */
  reformatting: boolean;
};

/** Initial panel state: nothing loaded, pretty view. */
export class PreviewStateFactory {
  static initial(): PreviewState {
    return {
      request: null,
      session: null,
      records: [],
      schema: null,
      loadPhase: PreviewPhase.Idle,
      loadFailure: null,
      result: null,
      runPhase: PreviewPhase.Idle,
      runFailure: null,
      testResult: null,
      testPhase: PreviewPhase.Idle,
      query: '',
      searchHits: null,
      selectedSeq: null,
      view: SampleView.Pretty,
      branch: null,
      reformatting: false,
    };
  }
}
