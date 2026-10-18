import type { Step, ValidationIssue } from '../../generated';

/** Endpoints of the Transform Studio assistant (cmd/api `internal/assist/http`). */
export const AssistEndpoint = {
  steps: '/assist/steps',
  explain: '/assist/explain',
  exportJs: '/assist/export-js',
  inferMapping: '/preview/infer-mapping',
} as const;

/** At most this many samples are sent with a proposal request (design NFR-3). */
export const MaxAssistSamples = 2;

/** Issue severity that blocks accepting a proposed step. */
export const BlockingSeverity = 'error';

/** POST /assist/steps body. `fields` are full event paths (`value.…`). */
export type ProposeRequestDto = { instruction: string; currentSteps: Step[]; fields: string[]; samples?: unknown[]; connectionId?: string };

/** POST /preview/infer-mapping body and response. */
export type InferRequestDto = { input: unknown; desiredOutput: unknown };
export type InferMappingDto = {
  steps: Step[];
  mappings: { target: string; source?: string; reason: string; confidence: number }[];
  unmapped: string[];
  issues: ValidationIssue[];
};

/** POST /assist/steps response: steps to append, one diff line per step, issues at `/steps/{i}`. */
export type ProposalDto = {
  proposalId: string;
  steps: Step[];
  diff: { added: string[]; removed: string[]; changed: string[] };
  issues: ValidationIssue[];
  provider: string;
};

/** POST /assist/explain body and response. */
export type ExplainRequestDto = { steps: Step[] };
export type ExplanationDto = { summary: string; bullets: string[]; provider: string };

/** POST /assist/export-js body and response (one js step). */
export type ExportJsRequestDto = { steps: Step[] };
export type ExportJsDto = { step: Step };
