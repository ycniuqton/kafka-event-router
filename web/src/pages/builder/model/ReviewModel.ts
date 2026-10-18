import { BranchSet, DestinationOptions, FilterExpressionBuilder, FilterGroups, FilterMode, RouteStatus, StepKindLabel, TopicDefaults, type Connection, type RouteDraft } from '../../../domain';
import { ConnectionOption, SourceSubscription } from './SourceRules';
import type { PipelinePreviewResult } from './PipelinePreview';
import { RailModel } from './RailModel';

/** The route read as a sentence: From …, keep events where …, reshape with …, send to …. */
export type ReviewSentence = { source: string; filter: string; transform: string; destination: string; branch: string | null };
export type ValidationCheck = { ok: boolean; text: string };
export type DryRunLine = { label: string; key: string; json: string };

/** View model of the Review & deploy step (spec §7.4). */
export class ReviewModel {
  static readonly dryRunLimit = 4;

  static sentence(draft: RouteDraft, connections: Connection[]): ReviewSentence {
    const enabled = draft.steps.filter((s) => s.enabled);
    const source = ConnectionOption.name(connections, draft.source.connectionId);
    const destination = ConnectionOption.name(connections, draft.destination.connectionId);
    return {
      source: `${source} / ${SourceSubscription.text(draft.source, ' + ')}`,
      filter: ReviewModel.filterText(draft),
      transform: enabled.length ? enabled.map((s) => StepKindLabel[s.kind] + (s.field ? ` ${s.field}` : '')).join(', ') : 'no changes',
      destination: `${destination} / ${draft.destination.topic || '—'}`,
      branch: ReviewModel.branchText(draft),
    };
  }

  /** `branch "eu" (1 of 2)` for multi-branch routes, else null. */
  static branchText(draft: RouteDraft): string | null {
    const count = BranchSet.count(draft);
    if (count < 2) return null;
    const state = BranchSet.state(draft);
    return `branch "${state.id}" (${state.index + 1} of ${count})`;
  }

  /** Sample-based checks from the live preview run (the server validation is listed separately). */
  static checks(preview: PipelinePreviewResult): ValidationCheck[] {
    if (!preview.ready) return [{ ok: true, text: 'Sample run pending — load samples to dry-run the route' }];
    const pass = preview.passIndexes.length;
    return [{ ok: pass > 0 || !preview.total, text: `Filter matches ${pass} of ${preview.total} samples` }, ReviewModel.transformCheck(preview)];
  }

  static dryRun(preview: PipelinePreviewResult): DryRunLine[] {
    return preview.outputs
      .map((out, i) => (out?.event ? { label: `#${i + 1}`, key: ReviewModel.text(out.event.key), json: JSON.stringify(out.event.value) } : null))
      .filter((line): line is DryRunLine => line !== null)
      .slice(0, ReviewModel.dryRunLimit);
  }

  /** Minimum the server needs before validation can pass. */
  static canDeploy(draft: RouteDraft): boolean {
    return BranchSet.all(draft).every((b) => !!b.destination.topic) && SourceSubscription.isSet(draft.source);
  }

  static deployTitle(draft: RouteDraft): string {
    return draft.routeId && draft.status !== RouteStatus.Draft ? 'Deploy changes' : 'Deploy route';
  }

  private static text(value: unknown): string {
    return typeof value === 'string' ? value : JSON.stringify(value ?? null);
  }

  private static filterText(draft: RouteDraft): string {
    if (draft.filter.mode === FilterMode.Expression) return draft.filter.expression;
    if (draft.filter.mode === FilterMode.Code) return ReviewModel.codeFilterText;
    if (FilterGroups.isEmpty(draft.filter)) return 'every event';
    return FilterExpressionBuilder.build(draft.filter).replace(ReviewModel.eventPrefix, '');
  }

  private static readonly eventPrefix = /event\.value\.|event\./g;
  private static readonly codeFilterText = 'the filter function returns true';

  private static transformCheck(preview: PipelinePreviewResult): ValidationCheck {
    const errors = preview.errorIndexes;
    if (!errors.length) return { ok: true, text: 'Transform ran on every matched sample' };
    const ids = errors.map((i) => i + 1).join(', #');
    return { ok: false, text: `Transform fails on ${RailModel.plural(errors.length, 'sample')} (#${ids}) — they would go to the error policy` };
  }
}

/** Destination rules: topic existence, new-topic partitions and the default dead-letter topic. */
export class DestinationRules {
  static readonly dlqPrefix = 'router.dlq.';
  static readonly newTopicPartitions = TopicDefaults.partitions;
  static readonly dlqFallback = 'route';
  static readonly avroLabel = 'Avro';

  static topicExists(connection: Connection | undefined, topic: string): boolean {
    return !!connection?.topics.some((t) => t.name === topic);
  }

  /** Default DLQ topic `router.dlq.<route>` (the Settings tab can set another one). */
  static dlqName(routeName: string): string {
    return `${DestinationRules.dlqPrefix}${routeName.trim() || DestinationRules.dlqFallback}`;
  }

  /** Output formats: same as source, JSON, Avro (P2, written through the destination's schema registry). */
  static formats(): readonly string[] {
    return DestinationOptions.format;
  }
}
