/** Filter operators: string operators and numeric operators from the visual builder. */
export const FilterOp = {
  Equals: 'equals',
  NotEquals: 'not equals',
  Contains: 'contains',
  StartsWith: 'starts with',
  InList: 'in list',
  Exists: 'exists',
  NumEquals: '=',
  NumNotEquals: '≠',
  GreaterThan: '>',
  LessThan: '<',
} as const;
export type FilterOp = (typeof FilterOp)[keyof typeof FilterOp];

export const StringFilterOps: FilterOp[] = [
  FilterOp.Equals,
  FilterOp.NotEquals,
  FilterOp.Contains,
  FilterOp.StartsWith,
  FilterOp.InList,
  FilterOp.Exists,
];
export const NumberFilterOps: FilterOp[] = [
  FilterOp.NumEquals,
  FilterOp.NumNotEquals,
  FilterOp.GreaterThan,
  FilterOp.LessThan,
  FilterOp.Exists,
];

export const FilterCombinator = { And: 'AND', Or: 'OR' } as const;
export type FilterCombinator = (typeof FilterCombinator)[keyof typeof FilterCombinator];

export const FilterMode = { Visual: 'visual', Expression: 'expr', Code: 'code' } as const;
export type FilterMode = (typeof FilterMode)[keyof typeof FilterMode];

/**
 * One visual condition: `field op value` (value kept as typed text). `literal` marks a JSON literal
 * value (`true`, `false`, `null`) compared by type rather than as text.
 */
export type FilterRow = { field: string; op: FilterOp; value: string; literal?: boolean };

/**
 * The route filter: visual rows plus nested sub-groups joined by one combinator, a JS expression, or (Code
 * mode) a full JS function whose source is kept in `expression`.
 * Sub-groups carry their own combinator, rows and groups (their mode and expression are unused).
 */
export type FilterGroup = {
  combinator: FilterCombinator;
  rows: FilterRow[];
  mode: FilterMode;
  expression: string;
  groups?: FilterGroup[];
};

/** Literal values a condition may compare against by type. */
export const FilterLiteral = { True: 'true', False: 'false', Null: 'null' } as const;

/** Rules over (nested) filter groups shared by the builder, evaluator and mappers. */
export class FilterGroups {
  static readonly maxDepth = 3;

  static children(group: FilterGroup): FilterGroup[] {
    return group.groups ?? [];
  }

  /** Number of conditions in the group and all its sub-groups. */
  static conditionCount(group: FilterGroup): number {
    return group.rows.length + FilterGroups.children(group).reduce((n, g) => n + FilterGroups.conditionCount(g), 0);
  }

  static isEmpty(group: FilterGroup): boolean {
    return FilterGroups.conditionCount(group) === 0;
  }

  /** A new empty sub-group with the opposite combinator (the usual reason to nest). */
  static newGroup(parent: FilterGroup, row: FilterRow): FilterGroup {
    const combinator = parent.combinator === FilterCombinator.And ? FilterCombinator.Or : FilterCombinator.And;
    return { combinator, rows: [row], mode: parent.mode, expression: '', groups: [] };
  }

  /** Parses a literal row value; non-literal text stays a string. */
  static literalValue(row: FilterRow): unknown {
    if (!row.literal) return row.value;
    if (row.value === FilterLiteral.True) return true;
    if (row.value === FilterLiteral.False) return false;
    return row.value === FilterLiteral.Null ? null : row.value;
  }
}

/** Result of evaluating a filter on one event; `rowResults` aligns with `rows`. */
export type FilterResult = { pass: boolean; rowResults: boolean[]; error?: string };
