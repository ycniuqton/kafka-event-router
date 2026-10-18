import { MaskMode } from '../transform';
import { ValueHasher, ValueMasker } from './Privacy';

export type TransformFn = (event: unknown, ctx: TransformContext) => unknown;
export type CompiledTransform = { fn?: TransformFn; error?: string };

/** The `ctx` object handed to user JavaScript steps. */
export type TransformContext = {
  mask: { email: (v: unknown) => string; last4: (v: unknown) => string; full: (v: unknown) => string };
  hash: (v: unknown) => string;
  now: () => string;
  formatDate: (d: unknown) => string;
  log: (...args: unknown[]) => void;
};

/** Compiles user code (cached) and builds the sandbox context. Preview-only, not a security boundary. */
export class JsSandbox {
  static readonly previewNow = '2026-10-13T09:41:00Z';
  static readonly dateLength = 10;
  private static readonly cache = new Map<string, CompiledTransform>();

  static compile(code: string): CompiledTransform {
    const cached = JsSandbox.cache.get(code);
    if (cached) return cached;
    const compiled = JsSandbox.compileFresh(code);
    JsSandbox.cache.set(code, compiled);
    return compiled;
  }

  static context(logs: string[]): TransformContext {
    return {
      mask: {
        email: (v) => ValueMasker.mask(v, MaskMode.Email),
        last4: (v) => ValueMasker.mask(v, MaskMode.Last4),
        full: (v) => ValueMasker.mask(v, MaskMode.Full),
      },
      hash: (v) => ValueHasher.hash(v),
      now: () => JsSandbox.previewNow,
      formatDate: (d) => String(d).slice(0, JsSandbox.dateLength),
      log: (...args) => logs.push(args.map((a) => (typeof a === 'object' ? JSON.stringify(a) : String(a))).join(' ')),
    };
  }

  private static compileFresh(code: string): CompiledTransform {
    try {
      const factory = new Function(`${code}\n;return typeof transform==="function"?transform:null;`);
      const fn = factory() as TransformFn | null;
      return fn ? { fn } : { error: 'No function named transform found' };
    } catch (e) {
      return { error: `Syntax: ${(e as Error).message}` };
    }
  }
}
