import type { ApiError } from './ApiError';
import { JsonPointer } from './JsonPointer';

/** Pointer prefix → form field name, for forms whose fields differ from the API body shape. */
export type FieldAliases = Readonly<Record<string, string>>;

/**
 * Maps problem JSON Pointers onto form field names. Unaliased pointers become dotted names
 * (`/bootstrapServers/0` → `bootstrapServers.0`); the longest matching alias prefix wins.
 */
export class FieldErrorMapper {
  private readonly aliases: [string, string][];

  constructor(aliases: FieldAliases = {}) {
    this.aliases = Object.entries(aliases).sort(([a], [b]) => b.length - a.length);
  }

  fieldOf(pointer: string): string {
    const alias = this.aliases.find(([prefix]) => JsonPointer.isWithin(pointer, prefix));
    return alias ? alias[1] : JsonPointer.toFieldName(pointer);
  }

  /** `{field → message}` for an error; empty for errors without field paths. */
  map(error: ApiError): Record<string, string> {
    const out: Record<string, string> = {};
    for (const [pointer, message] of Object.entries(error.fieldErrors())) out[this.fieldOf(pointer)] ??= message;
    return out;
  }
}
