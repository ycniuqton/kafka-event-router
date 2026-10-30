import type { Step, ValueSource } from '../../api/generated';
import { FilterCombinator, FilterMode, type FilterGroup } from '../filter';
import type { JsonValue } from '../json';
import { LookupMode, MaskMode, StepFactory, TransformStepKind, type TransformStep } from '../transform';
import { ConvertFailure, ConvertTarget, StepConstant, StepFieldList, ValueSourceKind, type MetadataName } from '../transformOptions';
import { FilterTreeModel } from './FilterTreeModel';
import { MapTreeCodec } from './MapTreeCodec';
import { MapStart } from '../mapper';
import { LookupStepDefaults } from '../lookupStep';
import { LookupSourceCodec } from './LookupSourceCodec';

/** Prefix of event value paths: builder steps name fields relative to `value`. */
export class EventPath {
  static readonly valuePrefix = 'value.';

  static toDocument(field: string): string {
    return EventPath.valuePrefix + field;
  }

  static fromDocument(path: string | undefined): string {
    if (!path) return '';
    return path.startsWith(EventPath.valuePrefix) ? path.slice(EventPath.valuePrefix.length) : path;
  }
}

/** Studio value source (kind + text) ↔ engine `valueSource`. */
export class ValueSourceCodec {
  static toDocument(step: Pick<TransformStep, 'sourceKind' | 'argument'>): ValueSource {
    const kind = step.sourceKind ?? ValueSourceKind.Constant;
    if (kind === ValueSourceKind.Field) return { from: 'field', path: EventPath.toDocument(step.argument) };
    if (kind === ValueSourceKind.Metadata) return { from: 'metadata', name: step.argument as MetadataName };
    if (kind === ValueSourceKind.Formula) return { from: 'formula', expr: step.argument };
    return { from: 'constant', value: StepConstant.parse(step.argument) };
  }

  static fromDocument(source: ValueSource | undefined): Partial<TransformStep> | null {
    if (!source) return null;
    if (source.from === 'field') return { sourceKind: ValueSourceKind.Field, argument: EventPath.fromDocument(source.path) };
    if (source.from === 'metadata') return { sourceKind: ValueSourceKind.Metadata, argument: source.name ?? '' };
    if (source.from === 'constant') return { sourceKind: ValueSourceKind.Constant, argument: StepConstant.format(source.value) };
    if (source.from === 'formula') return { sourceKind: ValueSourceKind.Formula, argument: source.expr ?? '' };
    return null;
  }
}

/** Drop-if condition (visual rows or expression) ↔ engine filter node. */
class ConditionCodec {
  static readonly empty: FilterGroup = { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '' };

  static toDocument(group: FilterGroup | undefined): Step['condition'] {
    return group ? FilterTreeModel.toNode(group) : undefined;
  }

  static fromDocument(node: Step['condition']): Partial<TransformStep> | null {
    const read = FilterTreeModel.fromNode(node);
    return read.exact ? { condition: read.group } : null;
  }
}

type StepCodec = {
  engine: Step['kind'];
  toStep: (s: TransformStep) => Step;
  /** Null when the engine step uses options the Studio cannot show (kept as-is by the draft mapper). */
  fromStep: (s: Step) => Partial<TransformStep> | null;
};

/** Option shapes shared by several step kinds (field lists, from → to, field + value source). */
class StepShape {
  static fieldPaths(s: TransformStep): [string, ...string[]] {
    return StepFieldList.of(s).map((f) => EventPath.toDocument(f)) as [string, ...string[]];
  }

  static list(s: Step): Partial<TransformStep> {
    const fields = (s.fields ?? []).map((f) => EventPath.fromDocument(f));
    return { fields, field: fields[0] ?? '' };
  }

  static fromTo(s: Step): Partial<TransformStep> {
    return { field: EventPath.fromDocument(s.from), argument: EventPath.fromDocument(s.to) };
  }

  static withSource(s: Step): Partial<TransformStep> | null {
    const source = ValueSourceCodec.fromDocument(s.source);
    return source && { ...source, field: EventPath.fromDocument(s.field) };
  }
}

/** Lookup step: key field, optional target, inline rows or a pinned library table, optional default; or a lookup source. */
class LookupCodec {
  static toStep(s: TransformStep): Step {
    if (s.lookupMode === LookupMode.Source) return { kind: 'lookup', lookup: LookupSourceCodec.toSpec(s.lookupSource ?? LookupStepDefaults.step()) };
    const step: Step = { kind: 'lookup', field: EventPath.toDocument(s.field) };
    if (s.argument.trim()) step.to = EventPath.toDocument(s.argument);
    if (s.fallback?.trim()) step.defaultValue = StepConstant.parse(s.fallback);
    if (s.lookupMode === LookupMode.Table) return { ...step, ref: s.lookupRef ?? '', version: s.lookupVersion ?? 1 };
    return { ...step, rows: (s.lookupRows ?? []).map((r) => ({ key: r.key, value: r.value })) };
  }

  static fromStep(s: Step): Partial<TransformStep> {
    if (s.lookup) return { lookupMode: LookupMode.Source, lookupSource: LookupSourceCodec.fromSpec(s.lookup), lookupRows: [] };
    const base: Partial<TransformStep> = { field: EventPath.fromDocument(s.field), argument: EventPath.fromDocument(s.to as string | undefined) };
    if (s.defaultValue !== undefined) base.fallback = StepConstant.format(s.defaultValue);
    if (s.ref) return { ...base, lookupMode: LookupMode.Table, lookupRef: s.ref, lookupVersion: s.version ?? 1, lookupRows: [] };
    return { ...base, lookupMode: LookupMode.Inline, lookupRows: (s.rows ?? []).map((r) => ({ key: r.key, value: r.value as JsonValue })) };
  }
}

/** Split array / wrap / unwrap: value paths only (engine `key.*` member paths stay as-is, not shown by the Studio). */
class StructureCodec {
  static splitArray(s: TransformStep): Step {
    const step: Step = { kind: 'split_array', field: EventPath.toDocument(s.field) };
    if (s.argument.trim()) step.to = EventPath.toDocument(s.argument);
    return step;
  }

  static fromSplitArray(s: Step): Partial<TransformStep> | null {
    if (!StructureCodec.inValue(s.field) || (s.to !== undefined && !StructureCodec.inValue(s.to as string))) return null;
    return { field: EventPath.fromDocument(s.field), argument: EventPath.fromDocument(s.to as string | undefined) };
  }

  static fromMember(s: Step): Partial<TransformStep> | null {
    return StructureCodec.inValue(s.field) ? { field: EventPath.fromDocument(s.field) } : null;
  }

  private static inValue(path: string | undefined): boolean {
    return !!path?.startsWith(EventPath.valuePrefix);
  }
}

/** Header add / remove / rename and header ↔ payload copies (`header`, `newHeader`, `field`, `source`). */
class HeaderCodec {
  static add(s: TransformStep): Step {
    return { kind: 'header_add', header: s.header ?? '', source: ValueSourceCodec.toDocument(s) };
  }

  static fromAdd(s: Step): Partial<TransformStep> | null {
    const source = ValueSourceCodec.fromDocument(s.source);
    return source && { ...source, header: s.header ?? '' };
  }

  static rename(s: TransformStep): Step {
    return { kind: 'header_rename', header: s.header ?? '', newHeader: s.argument };
  }

  static fromRename(s: Step): Partial<TransformStep> {
    return { header: s.header ?? '', argument: s.newHeader ?? '' };
  }

  static withField(kind: 'header_copy_to_payload' | 'header_copy_from_payload', s: TransformStep): Step {
    return { kind, header: s.header ?? '', field: EventPath.toDocument(s.field) };
  }

  static fromWithField(s: Step): Partial<TransformStep> {
    return { header: s.header ?? '', field: EventPath.fromDocument(s.field) };
  }
}

/** Converts builder transform steps to engine steps and back; unknown engine kinds return null. */
export class StepMapper {
  private static readonly codecs: Record<TransformStepKind, StepCodec> = {
    [TransformStepKind.Rename]: {
      engine: 'rename',
      toStep: (s) => ({ kind: 'rename', from: EventPath.toDocument(s.field), to: EventPath.toDocument(s.argument) }),
      fromStep: (s) => StepShape.fromTo(s),
    },
    [TransformStepKind.Remove]: { engine: 'remove', toStep: (s) => ({ kind: 'remove', fields: StepShape.fieldPaths(s) }), fromStep: (s) => StepShape.list(s) },
    [TransformStepKind.KeepOnly]: { engine: 'keep_only', toStep: (s) => ({ kind: 'keep_only', fields: StepShape.fieldPaths(s) }), fromStep: (s) => StepShape.list(s) },
    [TransformStepKind.Copy]: {
      engine: 'copy',
      toStep: (s) => ({ kind: 'copy', from: EventPath.toDocument(s.field), to: EventPath.toDocument(s.argument) }),
      fromStep: (s) => StepShape.fromTo(s),
    },
    [TransformStepKind.Move]: {
      engine: 'move',
      toStep: (s) => ({ kind: 'move', from: EventPath.toDocument(s.field), to: EventPath.toDocument(s.argument) }),
      fromStep: (s) => StepShape.fromTo(s),
    },
    [TransformStepKind.Mask]: {
      engine: 'mask',
      toStep: (s) => ({ kind: 'mask', field: EventPath.toDocument(s.field), style: s.maskMode }),
      fromStep: (s) => ({ field: EventPath.fromDocument(s.field), maskMode: (s.style ?? MaskMode.Full) as MaskMode }),
    },
    [TransformStepKind.Hash]: {
      engine: 'hash',
      toStep: (s) => ({ kind: 'hash', field: EventPath.toDocument(s.field) }),
      fromStep: (s) => ({ field: EventPath.fromDocument(s.field) }),
    },
    [TransformStepKind.ConvertType]: {
      engine: 'convert_type',
      toStep: (s) => ({
        kind: 'convert_type',
        field: EventPath.toDocument(s.field),
        targetType: s.targetType ?? ConvertTarget.Number,
        onFailure: s.onFailure ?? ConvertFailure.Error,
      }),
      fromStep: (s) => ({ field: EventPath.fromDocument(s.field), targetType: s.targetType, onFailure: s.onFailure ?? ConvertFailure.Error }),
    },
    [TransformStepKind.Set]: {
      engine: 'set',
      toStep: (s) => ({ kind: 'set', field: EventPath.toDocument(s.field), source: ValueSourceCodec.toDocument(s) }),
      fromStep: (s) => StepShape.withSource(s),
    },
    [TransformStepKind.DefaultIfMissing]: {
      engine: 'default_if_missing',
      toStep: (s) => ({ kind: 'default_if_missing', field: EventPath.toDocument(s.field), value: StepConstant.parse(s.argument) }),
      fromStep: (s) => ({ field: EventPath.fromDocument(s.field), sourceKind: ValueSourceKind.Constant, argument: StepConstant.format(s.value) }),
    },
    [TransformStepKind.DropIf]: {
      engine: 'drop_if',
      toStep: (s) => ({ kind: 'drop_if', condition: ConditionCodec.toDocument(s.condition) }),
      fromStep: (s) => ConditionCodec.fromDocument(s.condition),
    },
    [TransformStepKind.SetKey]: {
      engine: 'set_key',
      toStep: (s) => ({ kind: 'set_key', source: ValueSourceCodec.toDocument({ ...s, argument: StepMapper.keySource(s) }) }),
      fromStep: (s) => {
        const source = ValueSourceCodec.fromDocument(s.source);
        if (!source) return null;
        return source.sourceKind === ValueSourceKind.Field ? { sourceKind: ValueSourceKind.Field, field: source.argument } : source;
      },
    },
    [TransformStepKind.Map]: {
      engine: 'map',
      toStep: (s) => ({ kind: 'map', start: s.mapStart ?? MapStart.CopyInput, target: MapTreeCodec.toTarget(s.mapEntries ?? []) }),
      fromStep: (s) => {
        const mapEntries = MapTreeCodec.fromTarget(s.target);
        return mapEntries && { mapStart: s.start ?? MapStart.CopyInput, mapEntries };
      },
    },
    [TransformStepKind.Lookup]: { engine: 'lookup', toStep: (s) => LookupCodec.toStep(s), fromStep: (s) => LookupCodec.fromStep(s) },
    [TransformStepKind.SplitArray]: { engine: 'split_array', toStep: (s) => StructureCodec.splitArray(s), fromStep: (s) => StructureCodec.fromSplitArray(s) },
    [TransformStepKind.Wrap]: { engine: 'wrap', toStep: (s) => ({ kind: 'wrap', field: EventPath.toDocument(s.field) }), fromStep: (s) => StructureCodec.fromMember(s) },
    [TransformStepKind.Unwrap]: { engine: 'unwrap', toStep: (s) => ({ kind: 'unwrap', field: EventPath.toDocument(s.field) }), fromStep: (s) => StructureCodec.fromMember(s) },
    [TransformStepKind.HeaderAdd]: { engine: 'header_add', toStep: (s) => HeaderCodec.add(s), fromStep: (s) => HeaderCodec.fromAdd(s) },
    [TransformStepKind.HeaderRemove]: { engine: 'header_remove', toStep: (s) => ({ kind: 'header_remove', header: s.header ?? '' }), fromStep: (s) => ({ header: s.header ?? '' }) },
    [TransformStepKind.HeaderRename]: { engine: 'header_rename', toStep: (s) => HeaderCodec.rename(s), fromStep: (s) => HeaderCodec.fromRename(s) },
    [TransformStepKind.HeaderToPayload]: {
      engine: 'header_copy_to_payload',
      toStep: (s) => HeaderCodec.withField('header_copy_to_payload', s),
      fromStep: (s) => HeaderCodec.fromWithField(s),
    },
    [TransformStepKind.HeaderFromPayload]: {
      engine: 'header_copy_from_payload',
      toStep: (s) => HeaderCodec.withField('header_copy_from_payload', s),
      fromStep: (s) => HeaderCodec.fromWithField(s),
    },
    [TransformStepKind.Library]: {
      engine: 'library',
      toStep: (s) => ({ kind: 'library', ref: s.libraryRef ?? '', version: s.libraryVersion ?? 1 }),
      fromStep: (s) => ({ libraryRef: s.ref ?? '', libraryVersion: s.version ?? 1 }),
    },
    [TransformStepKind.Js]: { engine: 'js', toStep: (s) => ({ kind: 'js', code: s.code }), fromStep: (s) => ({ code: s.code ?? '' }) },
  };

  static readonly emptyCondition: FilterGroup = ConditionCodec.empty;

  static toStep(step: TransformStep): Step {
    const out = StepMapper.codecs[step.kind].toStep(step);
    if (!step.enabled) out.enabled = false;
    return out;
  }

  static fromStep(step: Step): TransformStep | null {
    const entry = (Object.entries(StepMapper.codecs) as [TransformStepKind, StepCodec][]).find(([, codec]) => codec.engine === step.kind);
    if (!entry) return null;
    const [kind, codec] = entry;
    const options = codec.fromStep(step);
    return options && StepFactory.create(kind, { ...options, enabled: step.enabled !== false });
  }

  /** Set key reads `field` when its source is a field (click-to-transform), else `argument`. */
  private static keySource(step: TransformStep): string {
    const kind = step.sourceKind ?? ValueSourceKind.Field;
    return kind === ValueSourceKind.Field ? step.field : step.argument;
  }
}
