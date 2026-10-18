import type { FilterGroup } from './filter';
import { MapStart, type MapEntry } from './mapper';
import { ConvertFailure, ConvertTarget, ValueSourceKind } from './transformOptions';
import type { LookupRow } from './library';

/** Kinds of transform step available in the Transform Studio (engine recipe kinds, see engineCatalog). */
export const TransformStepKind = {
  Rename: 'rename',
  Remove: 'remove',
  KeepOnly: 'keepOnly',
  Copy: 'copy',
  Move: 'move',
  Mask: 'mask',
  Hash: 'hash',
  ConvertType: 'convertType',
  Set: 'set',
  DefaultIfMissing: 'defaultIfMissing',
  DropIf: 'dropIf',
  SetKey: 'setKey',
  Map: 'map',
  Lookup: 'lookup',
  SplitArray: 'splitArray',
  Wrap: 'wrap',
  Unwrap: 'unwrap',
  HeaderAdd: 'headerAdd',
  HeaderRemove: 'headerRemove',
  HeaderRename: 'headerRename',
  HeaderToPayload: 'headerToPayload',
  HeaderFromPayload: 'headerFromPayload',
  Library: 'library',
  Js: 'js',
} as const;
export type TransformStepKind = (typeof TransformStepKind)[keyof typeof TransformStepKind];

export const MaskMode = { Email: 'email', Last4: 'last4', Full: 'full' } as const;

/** Where a lookup step reads its table: rows kept in the step, or a named library table pinned to a version. */
export const LookupMode = { Inline: 'inline', Table: 'table' } as const;
export type LookupMode = (typeof LookupMode)[keyof typeof LookupMode];
export type MaskMode = (typeof MaskMode)[keyof typeof MaskMode];

/**
 * One pipeline step. `field` is the value path the step acts on; `argument` is the rename/copy/move
 * target, or the value source text (constant, field path or metadata name). `fields` lists the paths of
 * remove/keep-only, `condition` is the drop-if condition (event paths). `code` is only used by JS steps.
 */
export type TransformStep = {
  id: number;
  kind: TransformStepKind;
  enabled: boolean;
  field: string;
  argument: string;
  maskMode: MaskMode;
  code: string;
  fields?: string[];
  sourceKind?: ValueSourceKind;
  targetType?: ConvertTarget;
  onFailure?: ConvertFailure;
  condition?: FilterGroup;
  /** Visual Mapper (`map` step): how the output starts and its output fields. */
  mapStart?: MapStart;
  mapEntries?: MapEntry[];
  /** Library step: the transform name and the pinned version (never updated automatically). */
  libraryRef?: string;
  libraryVersion?: number;
  /** Lookup step: `field` is the key, `argument` the target (empty = in place), rows inline or a pinned table. */
  lookupMode?: LookupMode;
  lookupRows?: LookupRow[];
  lookupRef?: string;
  lookupVersion?: number;
  /** Lookup default for keys missing from the table (typed constant text; empty = keep the value). */
  fallback?: string;
  /** Header steps: the record header name (rename puts the new name in `argument`). */
  header?: string;
  explained?: boolean;
};

export const StepKindLabel: Record<TransformStepKind, string> = {
  [TransformStepKind.Rename]: 'Rename field',
  [TransformStepKind.Remove]: 'Remove field',
  [TransformStepKind.KeepOnly]: 'Keep only',
  [TransformStepKind.Copy]: 'Copy field',
  [TransformStepKind.Move]: 'Move field',
  [TransformStepKind.Mask]: 'Mask',
  [TransformStepKind.Hash]: 'Hash (SHA-256)',
  [TransformStepKind.ConvertType]: 'Convert type',
  [TransformStepKind.Set]: 'Set value',
  [TransformStepKind.DefaultIfMissing]: 'Default if missing',
  [TransformStepKind.DropIf]: 'Drop if',
  [TransformStepKind.SetKey]: 'Set key from field',
  [TransformStepKind.Map]: 'Visual mapper',
  [TransformStepKind.Lookup]: 'Lookup',
  [TransformStepKind.SplitArray]: 'Split array',
  [TransformStepKind.Wrap]: 'Wrap',
  [TransformStepKind.Unwrap]: 'Unwrap',
  [TransformStepKind.HeaderAdd]: 'Add header',
  [TransformStepKind.HeaderRemove]: 'Remove header',
  [TransformStepKind.HeaderRename]: 'Rename header',
  [TransformStepKind.HeaderToPayload]: 'Copy header to payload',
  [TransformStepKind.HeaderFromPayload]: 'Copy payload to header',
  [TransformStepKind.Library]: 'Library transform',
  [TransformStepKind.Js]: 'JavaScript',
};

export const MaskModeLabel: Record<MaskMode, string> = {
  [MaskMode.Email]: 'Email',
  [MaskMode.Last4]: 'Keep last 4',
  [MaskMode.Full]: 'Full',
};

export type StepMenuGroup = { group: string; items: { kind: TransformStepKind; label: string }[] };

/** The "+ Add step" menu, grouped as in spec §8.2 (only kinds the engine runs; see engineCatalog recipes). */
export const StepMenu: StepMenuGroup[] = [
  {
    group: 'Fields',
    items: [
      { kind: TransformStepKind.Rename, label: 'Rename' },
      { kind: TransformStepKind.Remove, label: 'Remove' },
      { kind: TransformStepKind.KeepOnly, label: 'Keep only' },
      { kind: TransformStepKind.Copy, label: 'Copy' },
      { kind: TransformStepKind.Move, label: 'Move' },
      { kind: TransformStepKind.Map, label: 'Visual mapper' },
    ],
  },
  {
    group: 'Privacy',
    items: [
      { kind: TransformStepKind.Mask, label: 'Mask' },
      { kind: TransformStepKind.Hash, label: 'Hash (SHA-256)' },
    ],
  },
  { group: 'Types & formats', items: [{ kind: TransformStepKind.ConvertType, label: 'Convert type' }] },
  {
    group: 'Values',
    items: [
      { kind: TransformStepKind.Set, label: 'Set value' },
      { kind: TransformStepKind.DefaultIfMissing, label: 'Default if missing' },
      { kind: TransformStepKind.Lookup, label: 'Lookup' },
    ],
  },
  {
    group: 'Structure',
    items: [
      { kind: TransformStepKind.SplitArray, label: 'Split array' },
      { kind: TransformStepKind.Wrap, label: 'Wrap' },
      { kind: TransformStepKind.Unwrap, label: 'Unwrap' },
    ],
  },
  { group: 'Logic', items: [{ kind: TransformStepKind.DropIf, label: 'Drop if' }] },
  {
    group: 'Kafka',
    items: [
      { kind: TransformStepKind.SetKey, label: 'Set key from field' },
      { kind: TransformStepKind.HeaderAdd, label: 'Add header' },
      { kind: TransformStepKind.HeaderRemove, label: 'Remove header' },
      { kind: TransformStepKind.HeaderRename, label: 'Rename header' },
      { kind: TransformStepKind.HeaderToPayload, label: 'Copy header to payload' },
      { kind: TransformStepKind.HeaderFromPayload, label: 'Copy payload to header' },
    ],
  },
  { group: 'Library', items: [{ kind: TransformStepKind.Library, label: 'Library transform' }] },
  { group: 'Code', items: [{ kind: TransformStepKind.Js, label: 'JavaScript step' }] },
];

/** Default code of a new JavaScript step. */
export const DefaultJsStep = `// Return the new event, null to drop it, or an array to emit several.
function transform(event, ctx) {
  const v = event.value;
  return {
    ...event,
    value: {
      ...v,
      total: v.items.reduce((s, i) => s + i.price * i.qty, 0),
      processedAt: ctx.now(),
    },
  };
}`;

/** The field path a step writes: the target of rename / copy / move / lookup-to, else the step's field. */
export class StepTarget {
  static readonly fromTo: readonly TransformStepKind[] = [TransformStepKind.Rename, TransformStepKind.Copy, TransformStepKind.Move, TransformStepKind.Lookup, TransformStepKind.SplitArray];

  static of(step: TransformStep): string {
    return StepTarget.fromTo.includes(step.kind) && step.argument ? step.argument : step.field;
  }
}

/** One-line description of a step for proposal diffs ("Rename field: a → b"). */
export class StepLine {
  static of(step: TransformStep): string {
    const label = StepKindLabel[step.kind];
    const fields = step.fields?.length ? step.fields.join(', ') : step.field;
    const target = step.argument ? ` → ${step.argument}` : '';
    const ref = step.libraryRef ? ` ${step.libraryRef}` : '';
    return fields || target || ref ? `${label}: ${fields}${target}${ref}` : label;
  }
}

/** Creates steps with unique ids and the design's defaults (ids start after 100). */
export class StepFactory {
  private static lastId = 100;

  /** Extra defaults of kinds configured by more than a field and an argument. */
  static readonly kindDefaults: Partial<Record<TransformStepKind, Partial<TransformStep>>> = {
    [TransformStepKind.Set]: { sourceKind: ValueSourceKind.Constant },
    [TransformStepKind.DefaultIfMissing]: { sourceKind: ValueSourceKind.Constant },
    [TransformStepKind.SetKey]: { sourceKind: ValueSourceKind.Field },
    [TransformStepKind.HeaderAdd]: { sourceKind: ValueSourceKind.Constant, header: '' },
    [TransformStepKind.HeaderRemove]: { header: '' },
    [TransformStepKind.HeaderRename]: { header: '' },
    [TransformStepKind.HeaderToPayload]: { header: '' },
    [TransformStepKind.HeaderFromPayload]: { header: '' },
    [TransformStepKind.ConvertType]: { targetType: ConvertTarget.Number, onFailure: ConvertFailure.Error },
    [TransformStepKind.Map]: { mapStart: MapStart.CopyInput, mapEntries: [] },
    [TransformStepKind.Lookup]: { lookupMode: LookupMode.Inline, lookupRows: [] },
  };

  static create(kind: TransformStepKind, overrides: Partial<TransformStep> = {}): TransformStep {
    StepFactory.lastId += 1;
    return {
      id: StepFactory.lastId,
      kind,
      enabled: true,
      field: '',
      argument: '',
      maskMode: MaskMode.Email,
      code: DefaultJsStep,
      ...StepFactory.kindDefaults[kind],
      ...overrides,
    };
  }
}
