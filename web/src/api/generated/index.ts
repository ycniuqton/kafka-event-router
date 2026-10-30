/* Entry point of the types generated from pkg/config/schema (see `make gen`). Mapping FE drafts to RouteConfig is routes' job. */
export type {
  RouteConfig,
  Source,
  Branch,
  FilterNode,
  Step,
  Steps,
  ValueSource,
  Destination,
  ErrorPolicy,
  Limits,
  NodeType,
  OpName,
  StepKind,
  MaskStyle,
  FormatType,
  MapField,
  MapStart,
  LookupRows,
  LookupSpec,
  LookupRequest,
  LookupMapping,
  LookupCache,
  LookupTemplates,
  LookupMissPolicy,
  LookupErrorPolicy,
  RedactPreset,
  StringCase,
  DateLayout,
} from "./routeConfig";
export type {
  EventTrace,
  BranchTrace,
  StepTrace,
  NodeResult,
  PatchOp,
  TraceError,
  EventStatus,
  ValidationIssue,
  ValidationResult,
  IssueCode,
  Severity,
} from "./engineTypes";
export type {
  EngineCatalog,
  FieldType,
  OperandKind,
  RecipeGroup,
  CatalogFunction,
  CatalogFunctionParam,
  FormulaValueKind,
  FormulaFunctionCategory,
  FormulaOperator,
  CatalogLibrary,
} from "./engineCatalogTypes";
export { engineCatalog } from "./engineCatalog";
import { engineCatalog as catalog } from "./engineCatalog";
/** Formula function catalogue for Studio autocomplete (transform-studio#TS-118). */
export const formulaFunctions = catalog.formulaFunctions;
/** Library pin conventions (spliced trace step ids, lookup table names). */
export const libraryCatalog = catalog.library;
export { LibraryStepIds, type SplicedStepId } from "./libraryStepIds";
/* OpenAPI types (platform-core, `npm run api:gen` → schema.ts). */
export type { paths as ApiPaths, components as ApiComponents, operations as ApiOperations } from "./schema";
export type ApiSchemas = import("./schema").components["schemas"];
