import { MapEntryEditor, type MapEntry } from '../../../../domain';

/** Kinds of output tree rows. */
export const OutputRowType = { Group: 'group', Entry: 'entry' } as const;

/** One row of the output tree: an object group (derived from paths) or a mapped entry. */
export type OutputRow =
  | { type: typeof OutputRowType.Group; path: string; name: string; depth: number }
  | { type: typeof OutputRowType.Entry; path: string; name: string; depth: number; index: number; entry: MapEntry };

/** Output tree rows: entries sorted by path, with a group row for every parent object. */
export class OutputTreeModel {
  static rows(entries: readonly MapEntry[]): OutputRow[] {
    const order = entries.map((entry, index) => ({ entry, index })).sort((a, b) => a.entry.path.localeCompare(b.entry.path));
    const rows: OutputRow[] = [];
    const groups = new Set<string>();
    for (const { entry, index } of order) {
      const parents = MapEntryEditor.parentSegments(entry.path);
      parents.forEach((name, depth) => {
        const path = parents.slice(0, depth + 1).join(MapEntryEditor.separator);
        if (groups.has(path)) return;
        groups.add(path);
        rows.push({ type: OutputRowType.Group, path, name, depth });
      });
      rows.push({ type: OutputRowType.Entry, path: entry.path, name: entry.path.split(MapEntryEditor.separator).pop() ?? '', depth: parents.length, index, entry });
    }
    return rows;
  }
}
