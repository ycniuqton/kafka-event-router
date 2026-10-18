/** A resource paired with the ETag it was read with; send `etag` back as `If-Match` on writes. */
export type Versioned<T> = { value: T; etag: string | null };

/** Remembers the last ETag seen per resource key (usually the resource path) for later `If-Match`. */
export class EtagStore {
  private readonly etags = new Map<string, string>();

  remember(key: string, etag: string | null): void {
    if (etag) this.etags.set(key, etag);
    else this.etags.delete(key);
  }

  ifMatch(key: string): string | null {
    return this.etags.get(key) ?? null;
  }

  forget(key: string): void {
    this.etags.delete(key);
  }
}
