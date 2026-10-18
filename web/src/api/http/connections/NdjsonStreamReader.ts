/** Line framing of NDJSON streams. */
export const NdjsonFraming = { lineBreak: '\n' } as const;

/**
 * Reads a `fetch` body as newline-delimited JSON, yielding one parsed object per line.
 * Chunks may split lines anywhere; a trailing line without a newline is still delivered.
 */
export class NdjsonStreamReader<T> {
  private readonly body: ReadableStream<Uint8Array>;

  constructor(body: ReadableStream<Uint8Array>) {
    this.body = body;
  }

  async *[Symbol.asyncIterator](): AsyncIterator<T> {
    const reader = this.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    try {
      for (;;) {
        const { value, done } = await reader.read();
        buffer += decoder.decode(value, { stream: !done });
        const lines = buffer.split(NdjsonFraming.lineBreak);
        buffer = lines.pop() ?? '';
        yield* NdjsonStreamReader.parse<T>(lines);
        if (done) break;
      }
      yield* NdjsonStreamReader.parse<T>([buffer]);
    } finally {
      reader.releaseLock();
    }
  }

  private static *parse<T>(lines: string[]): Generator<T> {
    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed) yield JSON.parse(trimmed) as T;
    }
  }
}
