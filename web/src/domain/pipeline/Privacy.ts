import { MaskMode } from '../transform';

/** Masking rules used by Mask steps, the live tail and `ctx.mask`. */
export class ValueMasker {
  static readonly bullet = '•';
  static readonly keptDigits = 4;

  static mask(raw: unknown, mode: MaskMode): string {
    const text = String(raw ?? '');
    if (mode === MaskMode.Email) return ValueMasker.maskEmail(text);
    if (mode === MaskMode.Last4) return ValueMasker.maskKeepLast(text);
    return ValueMasker.bullet.repeat(ValueMasker.keptDigits);
  }

  private static maskEmail(text: string): string {
    const [user, domain] = text.split('@');
    const hidden = ValueMasker.bullet.repeat(3);
    return domain ? `${user[0]}${hidden}@${domain}` : hidden;
  }

  private static maskKeepLast(text: string): string {
    const hiddenCount = Math.max(0, text.length - ValueMasker.keptDigits);
    return ValueMasker.bullet.repeat(hiddenCount) + text.slice(-ValueMasker.keptDigits);
  }
}

/** Deterministic display hash (FNV-1a, shown as `sha256:xxxxxxxx…` like the design). */
export class ValueHasher {
  static readonly offsetBasis = 2166136261;
  static readonly prime = 16777619;
  static readonly hexWidth = 8;
  static readonly hexRadix = 16;

  static hash(raw: unknown): string {
    let h = ValueHasher.offsetBasis;
    for (const ch of String(raw)) {
      h ^= ch.charCodeAt(0);
      h = Math.imul(h, ValueHasher.prime);
    }
    const hex = (h >>> 0).toString(ValueHasher.hexRadix).padStart(ValueHasher.hexWidth, '0');
    return `sha256:${hex}…`;
  }
}
