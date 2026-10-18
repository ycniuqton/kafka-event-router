import type { Branch, RouteConfig, Step } from '../../api/generated';
import type { Destination, ErrorAction, HeaderMode, KeyMode, PartitioningMode, Source, StartMode } from '../../api/generated/routeConfig';
import { BranchSet } from '../branches';
import { BuilderStep, DeadLetterPolicy, DestinationOptions, DraftDefaults, SourceFormatType, StartPosition, type DraftBranch, type DraftDestination, type RouteDraft, type SourceValueFormat } from '../builder';
import type { RouteStatus } from '../route';
import { FilterTreeModel } from './FilterTreeModel';
import { StepMapper } from './StepMapper';

/** Fixed members of the MVP route document. */
export const RouteDocumentDefaults = { apiVersion: 'router/v1', branchId: 'main', format: 'json' } as const;

/** Per branch: what the builder showed when the document was loaded, so untouched parts keep their original form. */
export type BranchSnapshot = { filter: string; steps: string; filterExact: boolean; stepsExact: boolean };

/** The loaded document plus the first branch's keys (and every branch's, by id). */
export type DraftSnapshot = BranchSnapshot & { base: RouteConfig; branches?: Record<string, BranchSnapshot> };

export type DraftMeta = { routeId: string | null; name: string; status: RouteStatus };

/** Label ↔ engine value tables of the builder's segmented controls. */
class OptionTable<T extends string> {
  private readonly labels: readonly string[];
  private readonly values: readonly T[];

  constructor(labels: readonly string[], values: readonly T[]) {
    this.labels = labels;
    this.values = values;
  }

  value(label: string): T {
    return this.values[Math.max(0, this.labels.indexOf(label))];
  }

  label(value: T | undefined, fallback: string): string {
    const i = value === undefined ? -1 : this.values.indexOf(value);
    return i < 0 ? fallback : this.labels[i];
  }
}

/** Source value format ↔ the document's `format` (subject / message type only for registry formats). */
class SourceFormatMapper {
  private static readonly registryTypes: readonly string[] = [SourceFormatType.Avro, SourceFormatType.Protobuf];

  static toFormat(value: SourceValueFormat | undefined, base: Source['format'] | undefined): Source['format'] {
    if (!value) return base ?? { type: RouteDocumentDefaults.format };
    const out: Source['format'] = { type: value.type };
    if (!SourceFormatMapper.registryTypes.includes(value.type)) return out;
    if (value.subject?.trim()) out.subject = value.subject.trim();
    if (value.type === SourceFormatType.Protobuf && value.messageType?.trim()) out.messageType = value.messageType.trim();
    return out;
  }

  static fromFormat(format: Source['format'] | undefined): SourceValueFormat {
    return { type: (format?.type as SourceFormatType | undefined) ?? SourceFormatType.Json, subject: format?.subject ?? '', messageType: format?.messageType ?? '' };
  }
}

const keyModes = new OptionTable<KeyMode>(DestinationOptions.key, ['keep', 'field', 'none']);
const partitionings = new OptionTable<PartitioningMode>(DestinationOptions.partitioning, ['same_as_source', 'by_key', 'round_robin']);
const headerModes = new OptionTable<HeaderMode>(DestinationOptions.headers, ['keep_all', 'keep_selected', 'drop_all']);
const destinationFormats = new OptionTable<'same_as_source' | 'json' | 'avro'>(DestinationOptions.format, ['same_as_source', 'json', 'avro']);
const errorActions = new OptionTable<ErrorAction>(DestinationOptions.onError, ['skip', 'dlq', 'pause']);
const startModes = new OptionTable<StartMode>(
  [StartPosition.Latest, StartPosition.Earliest, StartPosition.DateTime, StartPosition.Offsets],
  ['latest', 'earliest', 'timestamp', 'offsets'],
);

/** Maps the builder's `RouteDraft` to the generated `RouteConfig` document and back. */
export class RouteDraftMapper {
  /** Document → builder draft (first branch edited, others parked) plus the snapshot of non-representable parts. */
  static fromDocument(doc: RouteConfig, meta: DraftMeta): { draft: RouteDraft; snapshot: DraftSnapshot } {
    const read = (doc.branches?.length ? doc.branches : [undefined]).map((b, i) => RouteDraftMapper.readBranch(b, i, doc));
    const [main] = read;
    const draft: RouteDraft = {
      routeId: meta.routeId,
      name: meta.name,
      status: meta.status,
      step: BuilderStep.Source,
      source: RouteDraftMapper.sourceOf(doc.source),
      filter: main.branch.filter,
      steps: main.branch.steps,
      destination: main.branch.destination,
      changes: [],
    };
    if (read.length > 1 || main.branch.id !== RouteDocumentDefaults.branchId) {
      draft.branching = { index: 0, id: main.branch.id, others: read.slice(1).map((r) => r.branch) };
    }
    const branches = Object.fromEntries(read.map((r) => [r.branch.id, r.snapshot]));
    return { draft, snapshot: { base: doc, ...main.snapshot, branches } };
  }

  /** Builder draft → document; per branch, untouched non-representable filter/steps are copied from the snapshot. */
  static toDocument(draft: RouteDraft, snapshot: DraftSnapshot | null = null): RouteConfig {
    const base = snapshot?.base;
    return {
      ...base,
      apiVersion: RouteDocumentDefaults.apiVersion,
      name: draft.name,
      source: RouteDraftMapper.source(draft, base?.source),
      branches: BranchSet.all(draft).map((b, i) => RouteDraftMapper.branchDoc(b, i, snapshot)) as RouteConfig['branches'],
      errorPolicy: { ...base?.errorPolicy, onError: errorActions.value(draft.destination.onError) },
    };
  }

  /** One document branch → builder branch and its snapshot keys. */
  private static readBranch(branch: Branch | undefined, index: number, doc: RouteConfig): { branch: DraftBranch; snapshot: BranchSnapshot } {
    const filter = FilterTreeModel.fromNode(branch?.filter ?? undefined);
    const mapped = (branch?.transform?.steps ?? []).map((s) => StepMapper.fromStep(s));
    const steps = mapped.filter((s) => s !== null);
    const id = branch?.id ?? (index === 0 ? RouteDocumentDefaults.branchId : `${BranchSet.idPrefix}${index + 1}`);
    return {
      branch: { id, filter: filter.group, steps, destination: RouteDraftMapper.destinationOf(branch?.destination, doc) },
      snapshot: { filter: JSON.stringify(filter.group), steps: JSON.stringify(steps), filterExact: filter.exact, stepsExact: steps.length === mapped.length },
    };
  }

  /** One builder branch → document branch (its stored base kept for members the builder does not edit). */
  private static branchDoc(b: DraftBranch, index: number, snapshot: DraftSnapshot | null): Branch {
    const base = snapshot?.base.branches?.find((x) => x.id === b.id) ?? (index === 0 ? snapshot?.base.branches?.[0] : undefined);
    const snap = snapshot?.branches?.[b.id] ?? (index === 0 && snapshot ? snapshot : undefined);
    const keepFilter = !!snap && !snap.filterExact && JSON.stringify(b.filter) === snap.filter;
    const keepSteps = !!snap && !snap.stepsExact && JSON.stringify(b.steps) === snap.steps;
    const filter = keepFilter ? (base?.filter ?? undefined) : FilterTreeModel.toNode(b.filter);
    const steps: Step[] = keepSteps ? (base?.transform?.steps ?? []) : b.steps.map((s) => StepMapper.toStep(s));
    const out: Branch = { ...base, id: b.id, destination: RouteDraftMapper.destination(b.destination, base?.destination) };
    if (filter) out.filter = filter;
    else delete out.filter;
    if (steps.length) out.transform = { ...base?.transform, steps };
    else delete out.transform;
    return out;
  }

  private static source(draft: RouteDraft, base: Source | undefined): Source {
    const mode = startModes.value(draft.source.start);
    const out: Source = {
      ...base,
      connectionId: draft.source.connectionId,
      topics: draft.source.topics.slice(),
      format: SourceFormatMapper.toFormat(draft.source.valueFormat, base?.format),
      startFrom: RouteDraftMapper.startFrom(mode, draft.source, base?.startFrom),
    };
    const group = draft.source.consumerGroup?.trim();
    if (group) out.consumerGroup = group;
    else delete out.consumerGroup;
    const pattern = draft.source.topicPattern?.trim();
    if (pattern) {
      out.topicPattern = pattern;
      delete out.topics;
    } else delete out.topicPattern;
    return out;
  }

  /** Keeps the stored instant while the mode is unchanged (a picked one wins); offsets come from the draft. */
  private static startFrom(mode: StartMode, source: RouteDraft['source'], base: Source['startFrom']): Source['startFrom'] {
    if (mode === 'timestamp') return { mode, at: source.startAt || (base?.mode === mode ? base.at : undefined) || new Date().toISOString() };
    if (mode === 'offsets') return { mode, offsets: { ...(source.offsets ?? (base?.mode === mode ? base.offsets : undefined) ?? {}) } };
    if (base?.mode === mode) return base;
    return { mode };
  }

  private static destination(d: DraftDestination, base: Destination | undefined): Destination {
    const keyMode = keyModes.value(d.key);
    const headerMode = headerModes.value(d.headers);
    const formatType = destinationFormats.value(d.format);
    const out: Destination = {
      ...base,
      connectionId: d.connectionId,
      topic: d.topic,
      key: keyMode === 'field' ? { mode: keyMode, field: d.keyField ?? base?.key?.field } : { mode: keyMode },
      partitioning: partitionings.value(d.partitioning),
      headers: RouteDraftMapper.headers(headerMode, d, base?.headers),
      format: formatType === 'avro' ? { ...base?.format, type: formatType, subject: d.schemaSubject ?? base?.format?.subject } : { type: formatType },
    };
    if (d.createTopic) out.createTopic = { ...base?.createTopic, partitions: d.createTopic.partitions };
    else if (d.createTopic === null) delete out.createTopic;
    return out;
  }

  private static headers(mode: HeaderMode, d: RouteDraft['destination'], base: Destination['headers']): NonNullable<Destination['headers']> {
    const out: NonNullable<Destination['headers']> = { mode };
    const keep = d.keepHeaders ?? base?.keep;
    const add = (d.addHeaders ?? base?.add ?? []).filter((h) => h.key.trim());
    if (mode === 'keep_selected' && keep?.length) out.keep = keep;
    if (add.length) out.add = add;
    return out;
  }

  private static sourceOf(source: Source | undefined): RouteDraft['source'] {
    const topics = source?.topics ?? [];
    return {
      connectionId: source?.connectionId ?? '',
      topics: topics.slice(),
      start: startModes.label(source?.startFrom?.mode, StartPosition.Latest) as StartPosition,
      format: DraftDefaults.sourceFormat,
      multi: topics.length > 1,
      startAt: source?.startFrom?.at,
      consumerGroup: source?.consumerGroup ?? '',
      topicPattern: source?.topicPattern ?? '',
      offsets: { ...(source?.startFrom?.offsets ?? {}) },
      valueFormat: SourceFormatMapper.fromFormat(source?.format),
    };
  }

  private static destinationOf(d: Destination | undefined, doc: RouteConfig): RouteDraft['destination'] {
    return {
      connectionId: d?.connectionId ?? '',
      topic: d?.topic ?? '',
      key: keyModes.label(d?.key?.mode, DestinationOptions.key[0]),
      partitioning: partitionings.label(d?.partitioning, DestinationOptions.partitioning[1]),
      headers: headerModes.label(d?.headers?.mode, DestinationOptions.headers[0]),
      format: destinationFormats.label(d?.format?.type as 'same_as_source' | 'json' | 'avro' | undefined, DestinationOptions.format[0]),
      onError: errorActions.label(doc.errorPolicy?.onError, DeadLetterPolicy),
      keyField: d?.key?.field ?? '',
      addHeaders: (d?.headers?.add ?? []).map((h) => ({ ...h })),
      keepHeaders: d?.headers?.keep ?? [],
      schemaSubject: d?.format?.subject ?? '',
      createTopic: d?.createTopic ? { partitions: d.createTopic.partitions } : null,
    };
  }
}
