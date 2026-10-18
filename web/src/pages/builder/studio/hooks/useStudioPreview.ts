import { useCallback, useEffect, useRef, useState } from 'react';
import { services } from '../../../../api';
import type { SampleEvent, TransformStep } from '../../../../domain';
import { StudioPreviewController, type StudioPreviewState } from '../preview/StudioPreviewController';
import type { StudioSampleSource } from '../preview/StudioRunPlan';

/** Settled state of a run started by `runNow` (null when the studio unmounted). */
export type StudioRunOutcome = StudioPreviewState | null;

/** Live preview of the Studio: re-runs (debounced) whenever the steps, samples or their source change; `runNow` skips the wait. */
export function useStudioPreview(
  steps: TransformStep[],
  samples: SampleEvent[],
  source: StudioSampleSource,
): StudioPreviewState & { runNow(): Promise<StudioRunOutcome> } {
  const [state, setState] = useState<StudioPreviewState>({ result: null, pending: false, failure: null });
  const controller = useRef<StudioPreviewController | null>(null);
  useEffect(() => {
    const c = new StudioPreviewController(services.preview, setState);
    controller.current = c;
    return () => c.dispose();
  }, []);
  useEffect(() => controller.current?.schedule(steps, samples, false, source), [steps, samples, source]);
  const runNow = useCallback(async (): Promise<StudioRunOutcome> => {
    const c = controller.current;
    if (!c) return null;
    c.schedule(steps, samples, true, source);
    return c.settled();
  }, [steps, samples, source]);
  return { ...state, runNow };
}
