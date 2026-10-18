import type { JsonObject, JsonValue } from '../json';
import type { SampleHeader, SampleRecord } from './types';

/** The JSON shape the engine traces a record in (Go `engine.Event.Snapshot`): step patches apply to it. */
export type EventDocument = {
  key: JsonValue;
  value: JsonValue;
  headers: SampleHeader[];
  topic?: string;
  partition?: number;
  offset?: number;
  timestamp?: string;
};

/** Builds and reads trace documents of sample records. */
export class EventDocuments {
  static of(record: SampleRecord): EventDocument {
    return {
      key: record.key,
      value: record.value,
      headers: record.headers,
      topic: record.topic,
      partition: record.partition,
      offset: record.offset,
      timestamp: record.timestamp,
    };
  }

  /** The record parts the panel shows in trees and raw text (key, headers, value). */
  static visible(doc: EventDocument): JsonObject {
    return { key: doc.key, headers: EventDocuments.headerMap(doc.headers), value: doc.value };
  }

  static headerMap(headers: readonly SampleHeader[] | undefined): Record<string, string> {
    return Object.fromEntries((headers ?? []).map((h) => [h.key, h.value]));
  }

  static asDocument(value: JsonValue, fallback: EventDocument): EventDocument {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) return fallback;
    const obj = value as JsonObject;
    const headers = Array.isArray(obj.headers) ? (obj.headers as SampleHeader[]) : fallback.headers;
    return { ...fallback, key: obj.key ?? null, value: obj.value ?? null, headers };
  }
}
