import type { SampleEvent, TransformStep } from '../../../domain';

/**
 * Props contract between the route builder and the Transform Studio (step=transform).
 * `./TransformStudio.tsx` exports the component as the named export `TransformStudio` (default export optional).
 */
export interface TransformStudioProps {
  steps: TransformStep[];
  onStepsChange(steps: TransformStep[], changeLabel?: string): void;
  samples: SampleEvent[];
  selectedSampleId: number | null;
  onSelectSample(id: number | null): void;
  /** True while the steps equal the last saved draft; step cards show an unsaved dot once they differ from it. */
  stepsSaved?: boolean;
  /** Saved route id: enables the route's test cases (a new, unsaved route has none). */
  routeId?: string | null;
  /** Source connection of the draft: AI assist may send masked samples of it. */
  connectionId?: string | null;
  /** Sample session the samples were copied from: previews are masked by its source (connection sensitivity + role). */
  sampleSessionId?: string | null;
}
