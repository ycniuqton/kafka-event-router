import type { JsonObject } from './json';
import { JsonFlattener } from './pipeline/JsonFlattener';
import { ValueSourceKind } from './transformOptions';

/** How the Visual Mapper's output starts before mappings apply (engine `mapStart`). */
export const MapStart = { CopyInput: 'copy_input', Empty: 'empty' } as const;
export type MapStart = (typeof MapStart)[keyof typeof MapStart];

export const MapStartLabel: Record<MapStart, string> = {
  [MapStart.CopyInput]: 'Copy input',
  [MapStart.Empty]: 'Empty',
};

/**
 * One output field of the Visual Mapper (`map` step): its dotted path under `value`, and either a value
 * source (`sourceKind` + `argument`, like set steps) or `remove` (drop the field from a copied input).
 */
export type MapEntry = { path: string; sourceKind: ValueSourceKind; argument: string; remove?: boolean };

/** Edits of the mapper's entry list; every method returns a new list (entries are immutable). */
export class MapEntryEditor {
  static readonly separator = '.';
  private readonly entries: readonly MapEntry[];

  constructor(entries: readonly MapEntry[]) {
    this.entries = entries;
  }

  /** Maps an input field to an output path (the input field's name at the root by default). */
  mapField(inputPath: string, outputPath: string = MapEntryEditor.leaf(inputPath)): MapEntry[] {
    const path = this.freePath(outputPath);
    return [...this.entries, { path, sourceKind: ValueSourceKind.Field, argument: inputPath }];
  }

  /** Adds a fixed-value field with a free name. */
  addConstant(name: string): MapEntry[] {
    return [...this.entries, { path: this.freePath(name), sourceKind: ValueSourceKind.Constant, argument: '' }];
  }

  /** Marks a copied input field as removed from the output. */
  removeInput(path: string): MapEntry[] {
    if (this.entries.some((e) => e.path === path && e.remove)) return [...this.entries];
    return [...this.entries.filter((e) => e.path !== path), { path, sourceKind: ValueSourceKind.Constant, argument: '', remove: true }];
  }

  update(index: number, patch: Partial<MapEntry>): MapEntry[] {
    return this.entries.map((e, i) => (i === index ? { ...e, ...patch } : e));
  }

  rename(index: number, name: string): MapEntry[] {
    const entry = this.entries[index];
    return this.update(index, { path: [...MapEntryEditor.parentSegments(entry.path), name].join(MapEntryEditor.separator) });
  }

  delete(index: number): MapEntry[] {
    return this.entries.filter((_, i) => i !== index);
  }

  /** Moves an output field into an object `group` at its current level. */
  nest(index: number, group: string): MapEntry[] {
    const entry = this.entries[index];
    const parent = MapEntryEditor.parentSegments(entry.path);
    return this.update(index, { path: [...parent, group, MapEntryEditor.leaf(entry.path)].join(MapEntryEditor.separator) });
  }

  /** Moves an output field one level up (out of its parent object). */
  unnest(index: number): MapEntry[] {
    const entry = this.entries[index];
    const parent = MapEntryEditor.parentSegments(entry.path);
    if (!parent.length) return [...this.entries];
    return this.update(index, { path: [...parent.slice(0, -1), MapEntryEditor.leaf(entry.path)].join(MapEntryEditor.separator) });
  }

  /** Last segment of a dotted path (the field's own name). */
  static leaf(path: string): string {
    return path.split(MapEntryEditor.separator).pop() ?? path;
  }

  static parentSegments(path: string): string[] {
    return path.split(MapEntryEditor.separator).slice(0, -1);
  }

  static depth(path: string): number {
    return MapEntryEditor.parentSegments(path).length;
  }

  private freePath(path: string): string {
    const taken = new Set(this.entries.map((e) => e.path));
    let candidate = path;
    for (let n = 2; taken.has(candidate); n++) candidate = `${path}_${n}`;
    return candidate;
  }
}

/** Which input leaves the mapper uses: field sources, and (copy input) every leaf it keeps. */
export class MappedFields {
  /** Input leaf paths that reach no output field (shown grey in the mapper). */
  static unmapped(input: JsonObject, start: MapStart, entries: readonly MapEntry[]): string[] {
    const leaves = JsonFlattener.flatten(input).filter((l) => !l.isBranch).map((l) => l.path);
    const used = entries.filter((e) => !e.remove && e.sourceKind === ValueSourceKind.Field).map((e) => e.argument);
    const removed = entries.filter((e) => e.remove).map((e) => e.path);
    return leaves.filter((leaf) => {
      if (used.some((u) => MappedFields.covers(u, leaf))) return false;
      if (start === MapStart.Empty) return true;
      return removed.some((r) => MappedFields.covers(r, leaf));
    });
  }

  /** True when `path` is `leaf` or one of its parents. */
  static covers(path: string, leaf: string): boolean {
    return leaf === path || leaf.startsWith(path + MapEntryEditor.separator);
  }
}
