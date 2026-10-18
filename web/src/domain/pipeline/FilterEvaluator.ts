import { FilterCombinator, FilterGroups, FilterMode, FilterOp, type FilterGroup, type FilterResult, type FilterRow } from '../filter';
import { FieldPath } from './FieldPath';
import { FilterCode } from './FilterCode';

type Predicate = (actual: unknown, expected: string) => boolean;

/** Truth table of every filter operator (actual value from the event vs typed value). */
class FilterOperators {
  static readonly listSeparator = ',';

  static readonly predicates: Record<FilterOp, Predicate> = {
    [FilterOp.Equals]: (x, v) => String(x) === v,
    [FilterOp.NotEquals]: (x, v) => String(x) !== v,
    [FilterOp.Contains]: (x, v) => String(x ?? '').includes(v),
    [FilterOp.StartsWith]: (x, v) => String(x ?? '').startsWith(v),
    [FilterOp.InList]: (x, v) => v.split(FilterOperators.listSeparator).map((s) => s.trim()).includes(String(x)),
    [FilterOp.Exists]: (x) => x !== undefined,
    [FilterOp.NumEquals]: (x, v) => Number(x) === Number(v),
    [FilterOp.NumNotEquals]: (x, v) => Number(x) !== Number(v),
    [FilterOp.GreaterThan]: (x, v) => Number(x) > Number(v),
    [FilterOp.LessThan]: (x, v) => Number(x) < Number(v),
  };
}

/** Evaluates a route filter (visual rows and nested groups, or a JS expression) against one event. */
export class FilterEvaluator {
  static evaluate(event: unknown, filter: FilterGroup): FilterResult {
    if (filter.mode === FilterMode.Expression) return FilterEvaluator.evaluateExpression(event, filter.expression);
    if (filter.mode === FilterMode.Code) return FilterEvaluator.evaluateExpression(event, FilterCode.wrap(filter.expression));
    if (FilterGroups.isEmpty(filter)) return { pass: true, rowResults: [] };
    const rowResults = filter.rows.map((row) => FilterEvaluator.evaluateRow(event, row));
    return { pass: FilterEvaluator.evaluateGroup(event, filter), rowResults };
  }

  /** A group's verdict: its rows and sub-groups joined by its combinator (empty sub-groups are ignored). */
  static evaluateGroup(event: unknown, group: FilterGroup): boolean {
    const nested = FilterGroups.children(group).filter((g) => !FilterGroups.isEmpty(g));
    const results = [...group.rows.map((row) => FilterEvaluator.evaluateRow(event, row)), ...nested.map((g) => FilterEvaluator.evaluateGroup(event, g))];
    if (!results.length) return true;
    return group.combinator === FilterCombinator.And ? results.every(Boolean) : results.some(Boolean);
  }

  static evaluateRow(event: unknown, row: FilterRow): boolean {
    const actual = FieldPath.get(event, row.field);
    if (row.literal && (row.op === FilterOp.Equals || row.op === FilterOp.NotEquals)) {
      return (actual === FilterGroups.literalValue(row)) === (row.op === FilterOp.Equals);
    }
    const predicate = FilterOperators.predicates[row.op];
    return predicate ? predicate(actual, row.value) : true;
  }

  private static evaluateExpression(event: unknown, expression: string): FilterResult {
    try {
      const fn = new Function('event', 'ctx', `return (${expression});`);
      return { pass: !!fn(event, {}), rowResults: [] };
    } catch (e) {
      return { pass: false, rowResults: [], error: (e as Error).message };
    }
  }
}
