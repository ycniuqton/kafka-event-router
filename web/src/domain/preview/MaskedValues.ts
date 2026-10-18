import { MaskToken } from './types';

/** Recognises values the preview service masked (type-preserving strings and redaction markers). */
export class MaskedValues {
  private static readonly tokens: ReadonlySet<unknown> = new Set<unknown>(Object.values(MaskToken));

  /** True for a masked string leaf (masked numbers read as 0 and cannot be told apart). */
  static isMasked(value: unknown): boolean {
    return MaskedValues.tokens.has(value);
  }

  /** True when any leaf of `value` is a masked token. */
  static containsMasked(value: unknown): boolean {
    if (MaskedValues.isMasked(value)) return true;
    if (Array.isArray(value)) return value.some((v) => MaskedValues.containsMasked(v));
    if (value && typeof value === 'object') return Object.values(value).some((v) => MaskedValues.containsMasked(v));
    return false;
  }
}
