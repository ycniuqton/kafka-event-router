import { EventDocuments, type EventDocument, type JsonObject, type JsonValue, type KafkaEvent, type PreviewOutput, type SampleEvent } from '../../../../domain';

/** Converts preview trace documents and outputs into the Studio's event shape (key text, header map, object value). */
export class TraceEvent {
  static readonly wrappedValueKey = 'value';

  static fromDocument(doc: JsonValue, fallback: KafkaEvent): KafkaEvent {
    const input = TraceEvent.document(fallback);
    const read = EventDocuments.asDocument(doc, input);
    return { key: TraceEvent.text(read.key), headers: EventDocuments.headerMap(read.headers), value: TraceEvent.object(read.value) };
  }

  static fromOutput(output: PreviewOutput): KafkaEvent {
    return { key: TraceEvent.text(output.key), headers: EventDocuments.headerMap(output.headers), value: TraceEvent.object(output.value) };
  }

  /** The trace document of a sample: the shape step patches apply to. */
  static document(event: KafkaEvent | SampleEvent): EventDocument {
    const headers = Object.entries(event.headers ?? {}).map(([key, value]) => ({ key, value }));
    return { key: event.key === '' ? null : event.key, value: event.value as JsonValue, headers };
  }

  private static text(key: JsonValue): string {
    if (key === null || key === undefined) return '';
    return typeof key === 'string' ? key : JSON.stringify(key);
  }

  private static object(value: JsonValue): JsonObject {
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) return value;
    return { [TraceEvent.wrappedValueKey]: value };
  }
}
