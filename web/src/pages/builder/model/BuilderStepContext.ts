import type { BuilderStep, Connection, RouteDraft, SampleEvent } from '../../../domain';
import type { DestinationExtras } from './DestinationExtras';
import type { DirtyKey, DraftEditor } from './DraftSession';
import type { FilterFieldCatalog } from './FilterFieldCatalog';
import type { PipelinePreviewResult } from './PipelinePreview';
import type { BuilderPreview } from './useBuilderPreview';

/** Everything a step editor, the rail and the sample panel need from the builder workspace. */
export type BuilderStepContext = {
  draft: RouteDraft;
  dirty: ReadonlySet<DirtyKey>;
  editor: DraftEditor;
  step: BuilderStep;
  goTo(step: BuilderStep): void;
  samples: SampleEvent[];
  /** Live preview session of the draft (samples panel, server runs, test runs). */
  live: BuilderPreview;
  preview: PipelinePreviewResult;
  catalog: FilterFieldCatalog;
  connections: Connection[];
  extras: DestinationExtras;
  setExtras(patch: Partial<DestinationExtras>, change: string): void;
  whyIndex: number | null;
  setWhyIndex(index: number | null): void;
  selectedSampleId: number | null;
  selectSample(id: number | null): void;
  keepOnly(field: string, value: unknown): void;
  save(): void;
  runTest(): void;
  openDeploy(): void;
};
