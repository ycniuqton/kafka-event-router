import { BuilderStep, FilterGroups, FilterMode, LookupMode, TransformStepKind, type Connection, type RouteDraft } from '../../../domain';
import { LookupSourceCatalog } from '../studio/model/LookupSourceCatalog';
import { ConnectionOption, SourceSubscription } from './SourceRules';
import type { DirtyKey } from './DraftSession';
import type { PipelinePreviewResult } from './PipelinePreview';

/** One card of the pipeline rail. */
export type RailCardView = {
  step: BuilderStep;
  kind: string;
  summary: string;
  warning: string;
  dirty: boolean;
};

type CardSummary = { kind: string; summary: string; warning: string };
type SummaryContext = { draft: RouteDraft; preview: PipelinePreviewResult; connections: Connection[] };

/** Builds the Source → Filter → Transform → Destination cards with summaries and warnings. */
export class RailModel {
  static readonly steps: BuilderStep[] = [BuilderStep.Source, BuilderStep.Filter, BuilderStep.Transform, BuilderStep.Destination];

  private static readonly summarizers: Record<string, (ctx: SummaryContext) => CardSummary> = {
    [BuilderStep.Source]: ({ draft, connections }) => ({
      kind: 'Source',
      summary: `${ConnectionOption.name(connections, draft.source.connectionId)} / ${SourceSubscription.text(draft.source, ', ')}`,
      warning: SourceSubscription.isSet(draft.source) ? '' : 'Pick a topic',
    }),
    [BuilderStep.Filter]: (ctx) => RailModel.filterCard(ctx),
    [BuilderStep.Transform]: (ctx) => RailModel.transformCard(ctx),
    [BuilderStep.Destination]: ({ draft, connections }) => ({
      kind: 'Destination',
      summary: `${ConnectionOption.name(connections, draft.destination.connectionId)} / ${draft.destination.topic || '—'}`,
      warning: draft.destination.topic ? '' : 'Choose a topic',
    }),
  };

  static build(draft: RouteDraft, preview: PipelinePreviewResult, dirty: ReadonlySet<DirtyKey>, connections: Connection[] = []): RailCardView[] {
    return RailModel.steps.map((step) => ({ step, dirty: dirty.has(step), ...RailModel.summarizers[step]({ draft, preview, connections }) }));
  }

  static plural(count: number, noun: string): string {
    return `${count} ${noun}${count > 1 ? 's' : ''}`;
  }

  private static filterCard({ draft, preview }: SummaryContext): CardSummary {
    const rows = FilterGroups.conditionCount(draft.filter);
    const pass = preview.passIndexes.length;
    let summary = 'all events pass';
    if (draft.filter.mode === FilterMode.Expression) summary = 'JS expression';
    else if (draft.filter.mode === FilterMode.Code) summary = 'JS function';
    else if (rows) summary = preview.ready && preview.total ? `${RailModel.plural(rows, 'condition')} · ${pass}/${preview.total} match` : RailModel.plural(rows, 'condition');
    const noMatch = rows > 0 && preview.ready && preview.total > 0 && !pass;
    return { kind: 'Filter', summary, warning: noMatch ? 'Matches 0 sample events' : '' };
  }

  private static transformCard({ draft, preview }: SummaryContext): CardSummary {
    const on = draft.steps.filter((s) => s.enabled).length;
    const js = draft.steps.some((s) => s.kind === TransformStepKind.Js) ? ' · JS' : '';
    const errors = preview.errorIndexes.length;
    return {
      kind: 'Transform',
      summary: on ? `${RailModel.plural(on, 'step')}${js}${RailModel.lookupNote(draft)}` : 'No changes · pass through',
      warning: errors ? `${RailModel.plural(errors, 'event')} fail` : '',
    };
  }

  /** ` · lookup crm-api (HTTP)` for enabled lookup source steps (the builder shows which sources a route calls). */
  static lookupNote(draft: RouteDraft): string {
    const ids = draft.steps.filter((s) => s.enabled && s.kind === TransformStepKind.Lookup && s.lookupMode === LookupMode.Source && s.lookupSource?.sourceId).map((s) => s.lookupSource?.sourceId ?? '');
    const labels = [...new Set(ids)].map((id) => LookupSourceCatalog.label(id));
    return labels.length ? `${RailModel.lookupPrefix}${labels.join(RailModel.lookupSeparator)}` : '';
  }

  static readonly lookupPrefix = ' · lookup ';
  static readonly lookupSeparator = ', ';
}
