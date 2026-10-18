import type { HelperDoc, SafetyRule } from '../../../domain';

/** Endpoints of js-sandbox (relative to the API root). */
export class SandboxEndpoint {
  static readonly runtimeInfo = '/sandbox/runtime-info';
  static readonly check = '/sandbox/check';
}

export type SandboxLimitsDto = {
  jsTimeoutMs: number;
  jsMemoryMb: number;
  jsMaxOutputKb: number;
  logKbPerCall?: number;
};

export type RuntimeInfoDto = {
  engine: string;
  determinismVersion: number;
  limits: { defaults: SandboxLimitsDto; maxima: SandboxLimitsDto };
  helpers: HelperDoc[] | null;
  typings: string;
  safetyRules: SafetyRule[] | null;
};

export type CheckRequestDto = { code: string; entry: 'transform' | 'predicate' };

export type DiagnosticDto = {
  severity: 'error' | 'warning';
  message: string;
  line?: number;
  column?: number;
  path?: string;
};

export type CheckResponseDto = { diagnostics: DiagnosticDto[] | null };
