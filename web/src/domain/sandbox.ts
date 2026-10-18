/** JS sandbox runtime facts served by `GET /api/v1/sandbox/runtime-info` (js-sandbox). */

/** Per-call limits of JS steps; `logKbPerCall` is only set on defaults. */
export type SandboxLimits = {
  jsTimeoutMs: number;
  jsMemoryMb: number;
  jsMaxOutputKb: number;
  logKbPerCall: number | null;
};

/** One `ctx` helper as documented in the JS editor info panel. */
export type HelperDoc = { name: string; signature: string; doc: string };

/** One sandbox safety rule (UI spec §8.8), keyed by a stable id. */
export type SafetyRule = { id: string; text: string };

/** How serious a diagnostic is: errors block deploys, warnings only inform. */
export type DiagnosticSeverity = 'error' | 'warning';

/** The contract user code is checked against: a `transform` function or a one-line `predicate` expression. */
export type SandboxEntry = 'transform' | 'predicate';

/** A compile or runtime problem in user code, 1-based line/column (null when unknown), at a JSON Pointer of the route. */
export type Diagnostic = {
  severity: DiagnosticSeverity;
  message: string;
  line: number | null;
  column: number | null;
  path: string | null;
};

/** Effective sandbox facts: engine, determinism contract, limits, helper docs, Monaco typings and rules. */
export type RuntimeInfo = {
  engine: string;
  determinismVersion: number;
  limits: { defaults: SandboxLimits; maxima: SandboxLimits };
  helpers: HelperDoc[];
  typings: string;
  safetyRules: SafetyRule[];
};

/** Short labels derived from runtime info for the JS step's sandbox badge. */
export class SandboxSummary {
  static readonly separator = ' · ';
  static readonly sandboxed = 'Sandboxed';
  static readonly noNetwork = 'no network';
  static readonly perEventSuffix = ' ms / event';

  /** "Sandboxed · no network · 50 ms / event" from the effective default timeout. */
  static badge(info: RuntimeInfo): string {
    const timeout = `${info.limits.defaults.jsTimeoutMs}${SandboxSummary.perEventSuffix}`;
    return [SandboxSummary.sandboxed, SandboxSummary.noNetwork, timeout].join(SandboxSummary.separator);
  }
}
