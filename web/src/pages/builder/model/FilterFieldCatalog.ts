import type { SampleField } from '../../../api';
import { FieldPath, FilterLiteral, FilterOp, NumberFilterOps, StringFilterOps, type FieldNode, type FieldSchema, type FilterRow, type SampleEvent } from '../../../domain';

export type ValueOption = { value: string; label: string };

/** JSON value types reported by the preview schema (mirror of Go `preview.JSONType`). */
export const FieldType = { String: 'string', Number: 'number', Boolean: 'boolean', Object: 'object', Array: 'array', Null: 'null' } as const;
export type FieldType = (typeof FieldType)[keyof typeof FieldType];

/** Operators offered for boolean fields (compared as JSON literals). */
const BooleanFilterOps: FilterOp[] = [FilterOp.Equals, FilterOp.NotEquals, FilterOp.Exists];

/** Fields offered by the condition builder, their operators and the values seen in samples. */
export class FilterFieldCatalog {
  static readonly maxValueOptions = 6;
  static readonly optionOps: FilterOp[] = [FilterOp.Equals, FilterOp.NotEquals];
  static readonly percent = 100;
  static readonly fallbackField = 'value.';
  private static readonly containerTypes: ReadonlySet<string> = new Set([FieldType.Object, FieldType.Array]);

  readonly paths: string[];
  private readonly numeric: Set<string>;
  private readonly samples: SampleEvent[];
  private readonly schemaNodes: Map<string, FieldNode>;
  private readonly schemaSamples: number;

  /** `schema` (preview service inference) supplies value counts; loaded samples are the fallback. */
  constructor(fields: SampleField[], samples: SampleEvent[], schema: FieldSchema | null = null) {
    this.paths = fields.map((f) => f.path);
    this.numeric = new Set(fields.filter((f) => f.numeric).map((f) => f.path));
    this.samples = samples;
    this.schemaNodes = FilterFieldCatalog.index(schema);
    this.schemaSamples = schema?.samples ?? 0;
  }

  private static index(schema: FieldSchema | null): Map<string, FieldNode> {
    const out = new Map<string, FieldNode>();
    const queue = schema ? [...schema.root.children] : [];
    while (queue.length) {
      const node = queue.shift() as FieldNode;
      out.set(node.path, node);
      queue.push(...node.children);
    }
    return out;
  }

  isNumeric(path: string): boolean {
    return this.numeric.has(path);
  }

  /** Most frequent JSON type of a field in the samples (schema), else number/string from the field list. */
  typeOf(path: string): FieldType {
    const types = Object.entries(this.schemaNodes.get(path)?.types ?? {}).filter(([t]) => t !== FieldType.Null);
    if (types.length) return types.sort((a, b) => b[1] - a[1])[0][0] as FieldType;
    return this.isNumeric(path) ? FieldType.Number : FieldType.String;
  }

  isBoolean(path: string): boolean {
    return this.typeOf(path) === FieldType.Boolean;
  }

  /** A sample value of the field for the picker ("OrderPaid", 42…), or empty. */
  example(path: string): string {
    const [first] = this.frequencies(path).keys();
    return first ?? '';
  }

  opsFor(path: string): FilterOp[] {
    if (this.isBoolean(path)) return BooleanFilterOps;
    return this.isNumeric(path) ? NumberFilterOps : StringFilterOps;
  }

  /** A new condition on the first known field (or an empty path to fill in). */
  newRow(): FilterRow {
    const [first] = this.conditionPaths('').filter(Boolean);
    return this.rowFor(first ?? FilterFieldCatalog.fallbackField);
  }

  /** Fields offered for a row, keeping a custom path (e.g. from a sample click) selectable. */
  pathsFor(path: string): string[] {
    return this.paths.includes(path) ? this.paths : [path, ...this.paths];
  }

  /** Scalar fields a condition can compare (objects and arrays are left out), keeping the row's own path. */
  conditionPaths(path: string): string[] {
    const scalar = this.paths
      .filter((p) => !FilterFieldCatalog.containerTypes.has(this.typeOf(p)))
      .sort((a, b) => Number(!a.startsWith(FilterFieldCatalog.fallbackField)) - Number(!b.startsWith(FilterFieldCatalog.fallbackField)));
    return scalar.includes(path) ? scalar : [path, ...scalar];
  }

  /** Distinct values of a field across samples with their counts. */
  frequencies(path: string): Map<string, number> {
    const node = this.schemaNodes.get(path);
    if (node?.topValues.length) return new Map(node.topValues.map((v) => [String(v.value), v.count]));
    const counts = new Map<string, number>();
    for (const ev of this.samples) {
      const value = FieldPath.get(ev, path);
      if (value !== undefined) counts.set(String(value), (counts.get(String(value)) ?? 0) + 1);
    }
    return counts;
  }

  /** Dropdown values ("OrderPaid — 33%") when the field has few distinct values, else null. */
  valueOptions(row: FilterRow): ValueOption[] | null {
    const counts = this.frequencies(row.field);
    if (counts.size > FilterFieldCatalog.maxValueOptions || !FilterFieldCatalog.optionOps.includes(row.op)) return null;
    const keys = [...counts.keys()];
    const values = keys.includes(row.value) ? keys : [row.value, ...keys];
    return values.map((value) => ({ value, label: this.optionLabel(value, counts.get(value)) }));
  }

  /** Row reset when its field changes: default operator for the type and the first seen value. */
  rowFor(path: string): FilterRow {
    const [first] = this.frequencies(path).keys();
    if (this.isBoolean(path)) return { field: path, op: FilterOp.Equals, value: first ?? FilterLiteral.True, literal: true };
    return { field: path, op: this.isNumeric(path) ? FilterOp.NumEquals : FilterOp.Equals, value: first ?? '' };
  }

  /** Row added from a clicked sample value ("Keep only events where …"); booleans and null compare as literals. */
  rowFromValue(path: string, value: unknown): FilterRow {
    if (typeof value === 'boolean' || value === null) return { field: path, op: FilterOp.Equals, value: String(value), literal: true };
    return { field: path, op: typeof value === 'number' ? FilterOp.NumEquals : FilterOp.Equals, value: String(value) };
  }

  private optionLabel(value: string, count: number | undefined): string {
    const total = this.schemaSamples || this.samples.length;
    if (!count || !total) return value;
    return `${value} — ${Math.round((count / total) * FilterFieldCatalog.percent)}%`;
  }
}
