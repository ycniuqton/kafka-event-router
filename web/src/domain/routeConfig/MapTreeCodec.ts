import type { MapField } from '../../api/generated';
import { MapEntryEditor, type MapEntry } from '../mapper';
import { ValueSourceKind } from '../transformOptions';
import { ValueSourceCodec } from './StepMapper';

/** Visual Mapper entries (dotted output paths) ↔ the engine's nested `map` target tree under `value`. */
export class MapTreeCodec {
  static toTarget(entries: readonly MapEntry[]): MapField {
    const root: MapField = {};
    for (const entry of entries) {
      if (!entry.path.trim()) continue;
      const node = MapTreeCodec.nodeAt(root, entry.path.split(MapEntryEditor.separator));
      if (entry.remove) node.remove = true;
      else node.source = ValueSourceCodec.toDocument(entry);
    }
    return root;
  }

  /** Entries of a target tree; null when a leaf uses a source the mapper cannot show. */
  static fromTarget(target: MapField | undefined): MapEntry[] | null {
    const out: MapEntry[] = [];
    return MapTreeCodec.walk(target ?? {}, [], out) ? out : null;
  }

  private static walk(node: MapField, at: string[], out: MapEntry[]): boolean {
    const path = at.join(MapEntryEditor.separator);
    if (node.remove) out.push({ path, sourceKind: ValueSourceKind.Constant, argument: '', remove: true });
    if (node.source) {
      const source = ValueSourceCodec.fromDocument(node.source);
      if (!source?.sourceKind) return false;
      out.push({ path, sourceKind: source.sourceKind, argument: source.argument ?? '' });
    }
    return Object.entries(node.fields ?? {}).every(([name, child]) => MapTreeCodec.walk(child, [...at, name], out));
  }

  private static nodeAt(root: MapField, segments: string[]): MapField {
    let node = root;
    for (const name of segments) {
      node.fields ??= {};
      node.fields[name] ??= {};
      node = node.fields[name];
    }
    return node;
  }
}
