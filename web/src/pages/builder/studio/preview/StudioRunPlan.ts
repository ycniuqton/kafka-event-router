import type { RouteConfig, Step } from '../../../../api/generated';
import { StepMapper } from '../../../../domain/routeConfig/StepMapper';
import { RouteDocumentDefaults } from '../../../../domain/routeConfig/RouteDraftMapper';
import type { InlineEvent, RunRequest, SampleEvent, TransformStep } from '../../../../domain';
import { StepCompleteness } from './StepCompleteness';

/**
 * Fixed parts of the stand-alone route document a Studio run sends (only the transform matters).
 * `unboundConnectionId` fills the schema-required connection of a draft without a source connection; the
 * preview never reads it because masking follows `sampleSessionId`.
 */
export const StudioRunDocument = {
  name: 'transform-studio-preview',
  unboundConnectionId: 'unbound',
  topic: 'transform-studio-preview',
} as const;

/** Where the Studio samples come from: the sample session they were copied from and the draft's source connection. */
export type StudioSampleSource = { sessionId: string | null; connectionId: string | null };

export const NoSampleSource: StudioSampleSource = { sessionId: null, connectionId: null };

/**
 * What one Studio run sends to `/preview/run`: the runnable steps (incomplete or excluded ones are
 * left out and reported), the samples as inline events and the session they came from (masking follows
 * that session's source). `stepIndexes[i]` maps run step i to the draft.
 */
export class StudioRunPlan {
  readonly request: RunRequest;
  readonly stepIndexes: number[];
  readonly skipped: ReadonlyMap<number, string>;
  readonly samples: SampleEvent[];
  readonly source: StudioSampleSource;

  constructor(steps: TransformStep[], samples: SampleEvent[], source: StudioSampleSource = NoSampleSource, excluded: ReadonlyMap<number, string> = new Map()) {
    const skipped = new Map(excluded);
    steps.forEach((s, i) => {
      const reason = skipped.has(i) ? null : StepCompleteness.reason(s);
      if (reason) skipped.set(i, reason);
    });
    this.skipped = skipped;
    this.stepIndexes = steps.map((_, i) => i).filter((i) => !skipped.has(i));
    this.samples = samples;
    this.source = source;
    const runSteps = this.stepIndexes.map((i) => StepMapper.toStep(steps[i]));
    const request: RunRequest = { config: StudioRunPlan.document(runSteps, samples, source), events: samples.map((s) => StudioRunPlan.inline(s)) };
    if (source.sessionId) request.sampleSessionId = source.sessionId;
    this.request = request;
  }

  /** Draft step index of a run step index (null when out of range). */
  draftIndex(runIndex: number | null | undefined): number | null {
    if (runIndex === null || runIndex === undefined) return null;
    return this.stepIndexes[runIndex] ?? null;
  }

  /** The same plan with one more step left out (after the preview rejected it). */
  without(steps: TransformStep[], draftIndex: number, reason: string): StudioRunPlan {
    return new StudioRunPlan(steps, this.samples, this.source, new Map([...this.skipped, [draftIndex, reason]]));
  }

  private static document(steps: Step[], samples: SampleEvent[], source: StudioSampleSource): RouteConfig {
    const topic = samples[0]?.topic || StudioRunDocument.topic;
    const transform = steps.length ? { steps } : undefined;
    const connectionId = source.connectionId || StudioRunDocument.unboundConnectionId;
    return {
      apiVersion: RouteDocumentDefaults.apiVersion,
      name: StudioRunDocument.name,
      source: { connectionId, topics: [topic], format: { type: RouteDocumentDefaults.format } },
      branches: [{ id: RouteDocumentDefaults.branchId, destination: { connectionId, topic: StudioRunDocument.topic }, transform }],
    };
  }

  private static inline(sample: SampleEvent): InlineEvent {
    const headers = Object.entries(sample.headers ?? {}).map(([key, value]) => ({ key, value }));
    const event: InlineEvent = { value: sample.value, headers, topic: sample.topic || undefined };
    if (sample.key) event.key = sample.key;
    if (sample.timestamp) event.timestamp = sample.timestamp;
    return event;
  }
}
