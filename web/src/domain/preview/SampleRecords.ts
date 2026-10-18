import type { SampleEvent } from '../event';
import type { JsonObject, JsonValue } from '../json';
import { EventDocuments } from './EventDocument';
import type { FieldNode, FieldSchema, SampleRecord } from './types';

/** A field offered by pickers: dotted path and whether numbers dominate. */
export type SchemaField = { path: string; numeric: boolean };

/** Converts preview records and schemas into the shapes filters, transforms and pickers use. */
export class SampleRecords {
  /** Member a non-object value is wrapped under, so filters always see an object payload. */
  static readonly wrappedValueKey = 'value';
  static readonly arrayMarker = '[]';
  static readonly numberType = 'number';

  static toEvent(record: SampleRecord): SampleEvent {
    return {
      id: record.seq,
      topic: record.topic,
      partition: record.partition,
      offset: record.offset,
      timestamp: record.timestamp,
      key: SampleRecords.text(record.key),
      headers: EventDocuments.headerMap(record.headers),
      value: SampleRecords.object(record.value) as SampleEvent['value'],
    };
  }

  static toEvents(records: readonly SampleRecord[]): SampleEvent[] {
    return records.map((r) => SampleRecords.toEvent(r));
  }

  /** Leaf fields of the inferred schema (dotted paths), numeric when numbers dominate. */
  static fields(schema: FieldSchema): SchemaField[] {
    const out: SchemaField[] = [];
    const queue: FieldNode[] = [...schema.root.children];
    while (queue.length > 0) {
      const node = queue.shift() as FieldNode;
      if (node.path.includes(SampleRecords.arrayMarker)) continue;
      if (node.children.length > 0) queue.push(...node.children);
      else out.push({ path: node.path, numeric: SampleRecords.numeric(node) });
    }
    return out;
  }

  /** Short one-line description of a record for list rows: first scalar members of the value. */
  static subtitle(record: SampleRecord, limit = 3): string {
    const coords = record.partition >= 0 ? `p${record.partition}@${record.offset}` : `#${record.offset}`;
    const value = record.value;
    if (value === null || typeof value !== 'object' || Array.isArray(value)) return `${JSON.stringify(value)} · ${coords}`;
    const scalars = Object.values(value).filter((v) => v === null || typeof v !== 'object').slice(0, limit);
    return [...scalars.map((v) => String(v)), coords].join(' · ');
  }

  /** Heading of a record row: the key, or the first string member of the value. */
  static title(record: SampleRecord): string {
    if (record.key !== null && record.key !== '') return SampleRecords.text(record.key);
    const value = record.value;
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const text = Object.values(value).find((v) => typeof v === 'string');
      if (typeof text === 'string') return text;
    }
    return record.topic;
  }

  private static numeric(node: FieldNode): boolean {
    const numbers = node.types[SampleRecords.numberType] ?? 0;
    return numbers > 0 && Object.values(node.types).every((count) => count <= numbers);
  }

  private static text(key: JsonValue): string {
    if (key === null) return '';
    return typeof key === 'string' ? key : JSON.stringify(key);
  }

  private static object(value: JsonValue): JsonObject {
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) return value;
    return { [SampleRecords.wrappedValueKey]: value };
  }
}
