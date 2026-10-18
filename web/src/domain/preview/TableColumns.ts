import { FieldPath } from '../pipeline/FieldPath';
import type { FieldNode, FieldSchema, SampleRecord } from './types';

export type TableColumn = { path: string; label: string };

/** Picks the Table view columns: the most frequent leaf fields of the value (schema first, samples otherwise). */
export class TableColumns {
  static readonly maxColumns = 6;
  static readonly valuePrefix = 'value.';
  static readonly arrayMarker = '[]';

  static fromSchema(schema: FieldSchema | null | undefined, records: readonly SampleRecord[]): TableColumn[] {
    const leaves = schema ? TableColumns.leaves(schema.root) : [];
    const paths = leaves.length ? leaves.sort((a, b) => b.frequency - a.frequency).map((n) => n.path) : TableColumns.sampled(records);
    return paths.slice(0, TableColumns.maxColumns).map((path) => ({ path, label: FieldPath.short(path) }));
  }

  /** Cell text of one record (objects rendered as compact JSON). */
  static cell(record: object, column: TableColumn): string {
    const v = FieldPath.get(record, column.path);
    if (v === undefined) return '';
    return typeof v === 'object' ? JSON.stringify(v) : String(v);
  }

  private static leaves(root: FieldNode): FieldNode[] {
    const out: FieldNode[] = [];
    const queue = [...root.children];
    while (queue.length) {
      const node = queue.shift() as FieldNode;
      if (node.path.includes(TableColumns.arrayMarker) || !node.path.startsWith(TableColumns.valuePrefix)) continue;
      if (node.children.length) queue.push(...node.children);
      else out.push(node);
    }
    return out;
  }

  private static sampled(records: readonly SampleRecord[]): string[] {
    const first = records.find((r) => r.value !== null && typeof r.value === 'object' && !Array.isArray(r.value));
    return first ? Object.keys(first.value as object).map((k) => TableColumns.valuePrefix + k) : [];
  }
}
