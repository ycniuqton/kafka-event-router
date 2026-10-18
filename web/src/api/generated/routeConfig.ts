/* generated from pkg/config/schema by make gen, do not edit */

/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "apiVersion".
 */
export type APIVersion = "router/v1";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "destinationFormatType".
 */
export type DestinationFormatType = "same_as_source" | "json" | "avro" | "protobuf" | "string" | "bytes";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "headerMode".
 */
export type HeaderMode = "keep_all" | "keep_selected" | "drop_all";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "keyMode".
 */
export type KeyMode = "keep" | "field" | "none";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "partitioningMode".
 */
export type PartitioningMode = "same_as_source" | "by_key" | "round_robin";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "opName".
 */
export type OpName =
  | "eq"
  | "ne"
  | "gt"
  | "gte"
  | "lt"
  | "lte"
  | "between"
  | "in"
  | "not_in"
  | "contains"
  | "starts_with"
  | "ends_with"
  | "regex"
  | "exists"
  | "not_exists"
  | "is_empty"
  | "before"
  | "after"
  | "within_last";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "nodeType".
 */
export type NodeType = "and" | "or" | "not" | "cond" | "expr";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "durationUnit".
 */
export type DurationUnit = "seconds" | "minutes" | "hours" | "days";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "stringCase".
 */
export type StringCase = "upper" | "lower" | "title" | "trim";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "stepKind".
 */
export type StepKind =
  | "rename"
  | "remove"
  | "keep_only"
  | "copy"
  | "move"
  | "set"
  | "default_if_missing"
  | "set_key"
  | "mask"
  | "hash"
  | "convert_type"
  | "drop_if"
  | "js"
  | "library"
  | "flatten"
  | "redact_pattern"
  | "format_date"
  | "round"
  | "string_case"
  | "lookup"
  | "if_else"
  | "split_array"
  | "wrap"
  | "unwrap"
  | "header_add"
  | "header_remove"
  | "header_rename"
  | "header_copy_to_payload"
  | "header_copy_from_payload"
  | "map";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "dateLayout".
 */
export type DateLayout = "rfc3339" | "rfc3339_ms" | "date" | "datetime";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "convertFailure".
 */
export type ConvertFailure = "error" | "null" | "keep";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "redactPreset".
 */
export type RedactPreset = "credit_card" | "phone" | "email" | "iban";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "lookupRows".
 */
export type LookupRows = LookupRow[];
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "valueSourceKind".
 */
export type ValueSourceKind = "field" | "constant" | "formula" | "metadata";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "metadataName".
 */
export type MetadataName = "topic" | "partition" | "offset" | "timestamp" | "route_name" | "route_version";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "mapStart".
 */
export type MapStart = "copy_input" | "empty";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "maskStyle".
 */
export type MaskStyle = "full" | "last4" | "email";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "convertTarget".
 */
export type ConvertTarget = "string" | "number" | "integer" | "boolean";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "steps".
 */
export type Steps = Step[];
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "deliveryMode".
 */
export type DeliveryMode = "at_least_once" | "exactly_once";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "errorAction".
 */
export type ErrorAction = "dlq" | "skip" | "pause";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "formatType".
 */
export type FormatType = "json" | "avro" | "protobuf" | "string" | "bytes";
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "startMode".
 */
export type StartMode = "latest" | "earliest" | "timestamp" | "offsets";

/**
 * Route configuration document (router/v1): source, branches (filter, transform, destination), error policy, delivery and limits.
 */
export interface RouteConfig {
  apiVersion: APIVersion;
  /**
   * @minItems 1
   * @maxItems 16
   */
  branches: [Branch, ...Branch[]];
  delivery?: Delivery;
  errorPolicy?: ErrorPolicy;
  limits?: Limits;
  name: string;
  source: Source;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "branch".
 */
export interface Branch {
  destination: Destination;
  filter?: null | FilterNode;
  id: string;
  transform?: Transform;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "destination".
 */
export interface Destination {
  connectionId: string;
  createTopic?: CreateTopic;
  format?: DestinationFormat;
  headers?: HeaderPolicy;
  key?: KeyPolicy;
  partitioning?: PartitioningMode;
  topic: string;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "createTopic".
 */
export interface CreateTopic {
  partitions: number;
  retentionMs?: number;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "destinationFormat".
 */
export interface DestinationFormat {
  messageType?: string;
  subject?: string;
  type: DestinationFormatType;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "headerPolicy".
 */
export interface HeaderPolicy {
  add?: HeaderEntry[];
  keep?: string[];
  mode: HeaderMode;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "headerEntry".
 */
export interface HeaderEntry {
  key: string;
  value: string;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "keyPolicy".
 */
export interface KeyPolicy {
  field?: string;
  mode: KeyMode;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "filterNode".
 */
export interface FilterNode {
  amount?: number;
  child?: FilterNode;
  children?: FilterNode[];
  code?: string;
  field?: string;
  from?: unknown;
  op?: OpName;
  to?: unknown;
  type: NodeType;
  unit?: DurationUnit;
  value?: unknown;
  values?: unknown[];
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "transform".
 */
export interface Transform {
  steps?: Steps;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "step".
 */
export interface Step {
  case?: StringCase;
  code?: string;
  condition?: FilterNode;
  decimals?: number;
  defaultValue?: unknown;
  elseSteps?: Steps;
  /**
   * Disabled steps are skipped; absent means enabled.
   */
  enabled?: boolean;
  field?: string;
  /**
   * @minItems 1
   */
  fields?: [string, ...string[]];
  from?: string;
  header?: string;
  id?: string;
  kind: StepKind;
  layout?: DateLayout;
  newHeader?: string;
  onFailure?: ConvertFailure;
  /**
   * Custom RE2 pattern of redact_pattern.
   */
  pattern?: string;
  preset?: RedactPreset;
  /**
   * Library name: a transform (library step) or a lookup table (lookup step without rows).
   */
  ref?: string;
  /**
   * Replacement text of redact_pattern; absent means "[REDACTED]".
   */
  replacement?: string;
  rows?: LookupRows;
  /**
   * Key separator of flatten; absent means ".".
   */
  separator?: string;
  source?: ValueSource;
  start?: MapStart;
  /**
   * Missing fields raise a step error instead of a no-op; absent means false.
   */
  strict?: boolean;
  style?: MaskStyle;
  target?: MapField;
  targetType?: ConvertTarget;
  thenSteps?: Steps;
  /**
   * IANA time zone; absent means UTC.
   */
  timezone?: string;
  to?: string;
  value?: unknown;
  /**
   * Pinned library version of ref; library steps expand it inline at compile time.
   */
  version?: number;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "lookupRow".
 */
export interface LookupRow {
  key: string;
  value: unknown;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "valueSource".
 */
export interface ValueSource {
  expr?: string;
  from: ValueSourceKind;
  name?: MetadataName;
  path?: string;
  value?: unknown;
}
/**
 * One node of the map step target tree under value: a leaf with a value source, a removal, or nested fields.
 *
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "mapField".
 */
export interface MapField {
  fields?: {
    [k: string]: MapField;
  };
  remove?: boolean;
  source?: ValueSource;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "delivery".
 */
export interface Delivery {
  mode: DeliveryMode;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "errorPolicy".
 */
export interface ErrorPolicy {
  /**
   * Connection of the DLQ topic; absent means the source connection.
   */
  dlqConnectionId?: string;
  dlqTopic?: string;
  maxRetries?: number;
  onError: ErrorAction;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "limits".
 */
export interface Limits {
  jsMemoryMb?: number;
  jsTimeoutMs?: number;
  maxInFlight?: number;
  maxOutputBytes?: number;
  maxOutputsPerEvent?: number;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "source".
 */
export interface Source {
  connectionId: string;
  consumerGroup?: string;
  format: Format;
  keyFormat?: Format;
  startFrom?: StartFrom;
  topicPattern?: string | null;
  topics?: string[];
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "format".
 */
export interface Format {
  messageType?: string;
  subject?: string;
  type: FormatType;
}
/**
 * This interface was referenced by `RouteConfig`'s JSON-Schema
 * via the `definition` "startFrom".
 */
export interface StartFrom {
  at?: string;
  mode: StartMode;
  offsets?: {
    [k: string]: number;
  };
}
