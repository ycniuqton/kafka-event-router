import { useEffect, useMemo } from 'react';
import type { SampleField } from '../../../api';
import { DemoSamples, SampleRecords, type Connection, type RouteDraft, type SampleEvent } from '../../../domain';
import { RouteDraftMapper } from '../../../domain/routeConfig/RouteDraftMapper';
import { usePreviewSession, type PreviewActions, type PreviewState, type SampleSourceRef } from '../../../store';
import { PipelinePreview, type PipelinePreviewResult } from './PipelinePreview';

/** Live preview of the draft: samples, server run projected per sample, schema fields and source names. */
export type BuilderPreview = {
  state: PreviewState;
  actions: PreviewActions;
  source: SampleSourceRef | null;
  connectionName: string;
  samples: SampleEvent[];
  fields: SampleField[];
  pipeline: PipelinePreviewResult;
  config: unknown;
};

/** Keys deciding when samples reload (connection + topics + demo sandbox) and when the draft re-runs (config JSON). */
class PreviewKeys {
  static readonly separator = '\u0000';
  static readonly demoMark = 'demo';

  static source(draft: RouteDraft, connections: Connection[]): SampleSourceRef | null {
    const { connectionId, topics } = draft.source;
    if (!connectionId || !topics.length) return null;
    const demo = DemoSamples.applies(connections.find((c) => c.id === connectionId), topics);
    return { connectionId, topics: topics.slice(), demo };
  }

  static sourceKey(source: SampleSourceRef | null): string {
    if (!source) return '';
    return [source.demo ? PreviewKeys.demoMark : '', source.connectionId, ...source.topics].join(PreviewKeys.separator);
  }

  static parse(key: string): SampleSourceRef | null {
    if (!key) return null;
    const [mark, connectionId, ...topics] = key.split(PreviewKeys.separator);
    return { connectionId, topics, demo: mark === PreviewKeys.demoMark };
  }
}

/**
 * Loads samples for the draft's source (or the attached `?session=`), re-runs the draft on the preview
 * service after every edit (debounced, stale runs aborted) and exposes the result in the builder's shape.
 */
export function useBuilderPreview(draft: RouteDraft, sessionId: string | null, connections: Connection[], fallbackFields: SampleField[]): BuilderPreview {
  const { state, actions } = usePreviewSession();
  const sourceKey = PreviewKeys.sourceKey(PreviewKeys.source(draft, connections));
  const source = useMemo(() => PreviewKeys.parse(sourceKey), [sourceKey]);
  useEffect(() => {
    if (sessionId) void actions.attach(sessionId);
    else if (source) void actions.ensureLatest(source);
  }, [actions, sessionId, source]);
  const config = useMemo(() => RouteDraftMapper.toDocument(draft), [draft]);
  const configKey = JSON.stringify(config);
  const sessionKey = state.session?.id ?? '';
  useEffect(() => {
    if (sessionKey) actions.scheduleRun(JSON.parse(configKey));
  }, [actions, sessionKey, configKey]);
  const samples = useMemo(() => SampleRecords.toEvents(state.records), [state.records]);
  const fields = useMemo(() => (state.schema ? SampleRecords.fields(state.schema) : fallbackFields), [state.schema, fallbackFields]);
  const pipeline = useMemo(() => PipelinePreview.fromRun(state.records, state.result, draft), [state.records, state.result, draft]);
  const connectionName = connections.find((c) => c.id === draft.source.connectionId)?.name ?? draft.source.connectionId;
  return { state, actions, source, connectionName, samples, fields, pipeline, config };
}
