/* generated from pkg/config/schema by make gen, do not edit */

/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "errorType".
 */
export type ErrorType = "decode" | "decode_transient" | "step" | "step_timeout" | "output_invalid" | "limit";
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "eventStatus".
 */
export type EventStatus = "passed" | "filtered" | "error" | "dropped";
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "issueCode".
 */
export type IssueCode =
  | "schema_invalid"
  | "connection_not_found"
  | "connection_forbidden"
  | "field_unknown"
  | "operator_type_mismatch"
  | "path_invalid"
  | "regex_invalid"
  | "operand_invalid"
  | "formula_invalid"
  | "js_compile"
  | "library_ref_missing"
  | "library_update_available"
  | "topic_missing"
  | "exactly_once_cross_cluster"
  | "test_case_failed"
  | "limit_out_of_range"
  | "step_unsupported"
  | "format_unsupported"
  | "version_unsupported";
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "severity".
 */
export type Severity = "error" | "warning";

/**
 * Wire shapes produced by pkg/config and pkg/engine other than the route document: validation results and per-event traces.
 */
export interface EngineTypes {
  eventTrace?: EventTrace;
  validationResult?: ValidationResult;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "eventTrace".
 */
export interface EventTrace {
  branches: BranchTrace[];
  error: null | TraceError;
  offset: number;
  status: EventStatus;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "branchTrace".
 */
export interface BranchTrace {
  filter: NodeResult[];
  id: string;
  outputs: number;
  steps: StepTrace[];
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "nodeResult".
 */
export interface NodeResult {
  /**
   * JSON Pointer relative to the branch filter ("" = root)
   */
  node: string;
  result: boolean;
  warning?: string;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "stepTrace".
 */
export interface StepTrace {
  durationUs: number;
  error: string | null;
  id: string;
  kind: string;
  logs: string[];
  patch: PatchOp[];
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "patchOp".
 */
export interface PatchOp {
  from?: string;
  op: "add" | "remove" | "replace" | "move" | "copy" | "test";
  path: string;
  value?: unknown;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "traceError".
 */
export interface TraceError {
  message: string;
  path?: string;
  step?: string;
  type: ErrorType;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "validationResult".
 */
export interface ValidationResult {
  issues: ValidationIssue[];
  valid: boolean;
}
/**
 * This interface was referenced by `EngineTypes`'s JSON-Schema
 * via the `definition` "validationIssue".
 */
export interface ValidationIssue {
  code: IssueCode;
  message: string;
  /**
   * JSON Pointer into the route document
   */
  path: string;
  severity: Severity;
}
