import { FilterCombinator, FilterMode, FilterOp, type FilterGroup, type FilterRow } from './filter';
import { RouteStatus, TransformMode, type Route } from './route';
import { StepFactory, TransformStepKind, type TransformStep } from './transform';
import { FilterExpressionBuilder } from './pipeline/FilterExpressionBuilder';

/** Steps of the route builder; the Transform Studio is the `transform` step. */
export const BuilderStep = {
  Source: 'source',
  Filter: 'filter',
  Transform: 'transform',
  Destination: 'destination',
  Review: 'review',
} as const;
export type BuilderStep = (typeof BuilderStep)[keyof typeof BuilderStep];

export const StartPosition = {
  Latest: 'Latest',
  Earliest: 'Earliest',
  DateTime: 'Date / time',
  Offsets: 'Offsets',
} as const;
export type StartPosition = (typeof StartPosition)[keyof typeof StartPosition];

/** Option lists of the destination step's segmented controls (values are display labels). */
export const DestinationOptions = {
  key: ['Keep original', 'From field', 'None'],
  partitioning: ['Same as source', 'By key', 'Round-robin'],
  headers: ['Keep all', 'Keep selected', 'Drop all'],
  format: ['Same as source', 'JSON', 'Avro'],
  onError: ['Skip and log', 'Dead-letter topic', 'Pause route'],
} as const;
export const DeadLetterPolicy = 'Dead-letter topic';

export type DraftSource = {
  connectionId: string;
  topics: string[];
  start: StartPosition;
  format: string;
  multi: boolean;
  /** Start instant (ISO) when `start` is Date / time. */
  startAt?: string;
  /** Kafka consumer group; read-only by default, editable under Advanced. Empty: server default. */
  consumerGroup?: string;
  /** Regex subscription (P2); when set, `topics` is not used. */
  topicPattern?: string;
  /** Start offsets per partition number when `start` is Offsets (P2). */
  offsets?: Record<string, number>;
  /** Value format override (P2); absent means JSON. */
  valueFormat?: SourceValueFormat;
};

/** Wire formats of source values (engine `format.type`). */
export const SourceFormatType = { Json: 'json', Avro: 'avro', Protobuf: 'protobuf', String: 'string', Bytes: 'bytes' } as const;
export type SourceFormatType = (typeof SourceFormatType)[keyof typeof SourceFormatType];

/** Source value format with its schema registry subject / Protobuf message type. */
export type SourceValueFormat = { type: SourceFormatType; subject?: string; messageType?: string };

/** A header added to every output record (spec §7.3 "add new (key/value rows)"). */
export type HeaderRow = { key: string; value: string };

export type DraftDestination = {
  connectionId: string;
  topic: string;
  key: string;
  partitioning: string;
  headers: string;
  format: string;
  onError: string;
  /** Field the output key is taken from when `key` is "From field". */
  keyField?: string;
  /** Headers added to every output record; kept header names for "Keep selected". */
  addHeaders?: HeaderRow[];
  keepHeaders?: string[];
  /** Avro schema subject when the output format is Avro. */
  schemaSubject?: string;
  /** Create the topic on deploy when it is missing (partitions of the new topic). */
  createTopic?: { partitions: number } | null;
};

/** Editable route configuration held by the builder (design `configFor` without UI-only state). */
export type RouteDraft = {
  routeId: string | null;
  name: string;
  status: RouteStatus;
  step: BuilderStep;
  source: DraftSource;
  filter: FilterGroup;
  steps: TransformStep[];
  destination: DraftDestination;
  changes: string[];
  /** P2 branches: the edited branch lives in filter/steps/destination; the others wait here. Absent: one branch `main`. */
  branching?: BranchState;
};

/** One parallel path of a route: its own filter, transform steps and destination. */
export type DraftBranch = { id: string; filter: FilterGroup; steps: TransformStep[]; destination: DraftDestination };

/** Which branch the builder edits (`index` in document order, `id`) and the branches not being edited. */
export type BranchState = { index: number; id: string; others: DraftBranch[] };

/** Filter rows and steps pre-filled for an existing route. */
export type DraftPreset = { rows: FilterRow[]; steps: TransformStep[] };

export const RouteTemplateId = { Filter: 'filter', Partner: 'partner', Merge: 'merge', Blank: 'blank' } as const;
export type RouteTemplateId = (typeof RouteTemplateId)[keyof typeof RouteTemplateId];

/** Building blocks drawn on a template card preview. */
export const TemplateBlock = { Source: 'src', Filter: 'flt', Mask: 'msk', Destination: 'dst' } as const;
export type TemplateBlock = (typeof TemplateBlock)[keyof typeof TemplateBlock];

const paidOrderRow = (): FilterRow => ({ field: 'value.eventType', op: FilterOp.Equals, value: 'OrderPaid' });

/** A "New route" starting point: card content plus how it pre-fills a draft. */
export abstract class RouteTemplate {
  abstract readonly id: RouteTemplateId;
  abstract readonly title: string;
  abstract readonly description: string;
  abstract readonly blocks: TemplateBlock[];
  abstract apply(draft: RouteDraft): void;
}

class FilterForwardTemplate extends RouteTemplate {
  readonly id = RouteTemplateId.Filter;
  readonly title = 'Filter & forward';
  readonly description = 'Keep some events, send them elsewhere.';
  readonly blocks = [TemplateBlock.Source, TemplateBlock.Filter, TemplateBlock.Destination];
  apply(draft: RouteDraft): void {
    draft.name = 'New filter & forward';
    draft.filter.rows = [paidOrderRow()];
    draft.destination.topic = 'orders.paid';
  }
}

class PartnerTemplate extends RouteTemplate {
  readonly id = RouteTemplateId.Partner;
  readonly title = 'Publish to partner';
  readonly description = 'Filter, mask sensitive fields, send to an external cluster.';
  readonly blocks = [TemplateBlock.Source, TemplateBlock.Filter, TemplateBlock.Mask, TemplateBlock.Destination];
  apply(draft: RouteDraft): void {
    draft.name = 'New partner feed';
    draft.filter.rows = [paidOrderRow()];
    draft.steps = [
      StepFactory.create(TransformStepKind.Mask, { field: 'email' }),
      StepFactory.create(TransformStepKind.Hash, { field: 'cust_id' }),
    ];
    draft.destination.connectionId = 'partner-kafka';
    draft.destination.topic = 'orders-public';
  }
}

class MergeTemplate extends RouteTemplate {
  readonly id = RouteTemplateId.Merge;
  readonly title = 'Merge topics';
  readonly description = 'Pick several source topics into one destination.';
  readonly blocks = [TemplateBlock.Source, TemplateBlock.Source, TemplateBlock.Destination];
  apply(draft: RouteDraft): void {
    draft.name = 'New merge';
    draft.source.multi = true;
    draft.source.topics = ['order-events', 'payment-events'];
    draft.destination.topic = 'commerce.all';
  }
}

class BlankTemplate extends RouteTemplate {
  readonly id = RouteTemplateId.Blank;
  readonly title = 'Blank route';
  readonly description = 'Start empty and build every step yourself.';
  readonly blocks: TemplateBlock[] = [];
  apply(draft: RouteDraft): void {
    draft.destination.topic = '';
  }
}

/** All templates in display order, and lookup by id. */
export class TemplateCatalog {
  static readonly all: RouteTemplate[] = [
    new FilterForwardTemplate(),
    new PartnerTemplate(),
    new MergeTemplate(),
    new BlankTemplate(),
  ];

  static find(id: string | null | undefined): RouteTemplate | undefined {
    return TemplateCatalog.all.find((t) => t.id === id);
  }
}

/** Defaults used when a draft starts without an existing route. */
export const DraftDefaults = {
  name: 'Untitled route',
  sourceConnectionId: 'orders-prod',
  sourceTopic: 'order-events',
  sourceFormat: 'JSON (auto-detected)',
  destinationConnectionId: 'analytics-staging',
} as const;

/** Builds a builder draft from an optional route, its preset and an optional template. */
export class RouteDraftFactory {
  static create(route: Route | null, preset: DraftPreset | null, template?: RouteTemplate): RouteDraft {
    const draft = RouteDraftFactory.base(route);
    if (preset) {
      draft.filter.rows = preset.rows;
      draft.steps = preset.steps;
    }
    template?.apply(draft);
    draft.filter.expression = FilterExpressionBuilder.build(draft.filter);
    return draft;
  }

  private static base(route: Route | null): RouteDraft {
    return {
      routeId: route?.id ?? null,
      name: route?.name ?? DraftDefaults.name,
      status: route?.status ?? RouteStatus.Draft,
      step: BuilderStep.Source,
      source: {
        connectionId: route?.source.connectionId ?? DraftDefaults.sourceConnectionId,
        topics: route ? route.source.topics.slice() : [DraftDefaults.sourceTopic],
        start: StartPosition.Latest,
        format: DraftDefaults.sourceFormat,
        multi: route ? route.source.topics.length > 1 : false,
      },
      filter: { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '' },
      steps: [],
      destination: {
        connectionId: route?.destination.connectionId ?? DraftDefaults.destinationConnectionId,
        topic: route?.destination.topic ?? '',
        key: DestinationOptions.key[0],
        partitioning: DestinationOptions.partitioning[1],
        headers: DestinationOptions.headers[0],
        format: DestinationOptions.format[0],
        onError: DeadLetterPolicy,
      },
      changes: [],
    };
  }
}

/** Summaries a deployed draft contributes to its route row. */
export class DraftSummary {
  static filterSummary(filter: FilterGroup): string {
    if (filter.mode === FilterMode.Expression) return 'expression';
    if (filter.mode === FilterMode.Code) return 'filter function';
    const [first] = filter.rows;
    if (!first) return 'all events';
    const more = filter.rows.length > 1 ? ` +${filter.rows.length - 1}` : '';
    return `${first.field.replace('value.', '')} ${first.op} ${first.value}${more}`;
  }

  static transformMode(steps: TransformStep[]): TransformMode {
    if (steps.some((s) => s.kind === TransformStepKind.Js)) return TransformMode.Js;
    return steps.length ? TransformMode.Recipe : TransformMode.None;
  }
}
