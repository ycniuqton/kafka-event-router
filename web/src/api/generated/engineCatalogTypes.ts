/* Hand-maintained shape of engineCatalog.json (pkg/engine/catalog.Catalog); checked by `make check-gen` compile. */
import type {
  FormatType,
  MapStart,
  MaskStyle,
  MetadataName,
  OpName,
  RedactPreset,
  StepKind,
  StringCase,
  ValueSourceKind,
} from "./routeConfig";

export type FieldType = "string" | "number" | "boolean" | "date" | "object" | "array" | "null";
export type OperandKind = "none" | "value" | "values" | "range" | "duration";
export type RecipeGroup = "fields" | "values" | "privacy" | "types" | "flow" | "structure" | "kafka" | "code" | "library";
export type DateLayout = "rfc3339" | "rfc3339_ms" | "date" | "datetime";

export interface CatalogOperator {
  op: OpName;
  fieldTypes: FieldType[];
  operand: OperandKind;
  handlesMissing: boolean;
}

export interface CatalogCoercionRule {
  left: string;
  right: string;
  rule: string;
  warning?: string;
}

export interface CatalogRecipe {
  kind: StepKind;
  group: RecipeGroup;
}

export type FormulaValueKind = "any" | "string" | "number" | "integer" | "boolean";
export type FormulaFunctionCategory = "text" | "number" | "logic" | "date" | "privacy" | "lookup";
export type FormulaOperator = "==" | "!=" | "<=" | ">=" | "&&" | "||" | "+" | "-" | "*" | "/" | "%" | "<" | ">" | "!";

export interface CatalogFunctionParam {
  name: string;
  kind: FormulaValueKind;
  optional?: boolean;
  variadic?: boolean;
}

/** One formula function; `arity.max` is -1 for variadic functions. `call` is the autocomplete form, e.g. `round(number, decimals?)`. */
export interface CatalogFunction {
  name: string;
  call: string;
  arity: { min: number; max: number };
  params: CatalogFunctionParam[];
  returns: FormulaValueKind;
  category: FormulaFunctionCategory;
  description: string;
  example: string;
}

/** How library pins appear in steps, trace step ids and lookup table names (pipeline-engine PE-100/PE-101). */
export interface CatalogLibrary {
  pinnedKinds: StepKind[];
  versionSeparator: string;
  stepIdSeparator: string;
  stepIdForm: string;
  lookupTableNames: string[];
  maxDepth: number;
}

export interface EngineCatalog {
  version: number;
  operators: CatalogOperator[];
  operatorsByFieldType: Partial<Record<FieldType, OpName[]>>;
  coercion: CatalogCoercionRule[];
  recipes: CatalogRecipe[];
  recipeGroups: RecipeGroup[];
  maskStyles: MaskStyle[];
  valueSources: ValueSourceKind[];
  metadataNames: MetadataName[];
  dateLayouts: DateLayout[];
  formulaFunctions: CatalogFunction[];
  formulaOperators: FormulaOperator[];
  redactPresets: RedactPreset[];
  stringCases: StringCase[];
  mapStarts: MapStart[];
  sourceFormats: FormatType[];
  library: CatalogLibrary;
}
