/** Serialized payload size in bytes (UTF-8), shown under the output. */
export class EventSize {
  private static readonly encoder = new TextEncoder();

  static bytes(value: unknown): number {
    if (value == null) return 0;
    return EventSize.encoder.encode(JSON.stringify(value)).length;
  }
}
