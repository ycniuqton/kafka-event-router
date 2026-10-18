import { FilterCombinator, FilterGroups, FilterOp, type FilterGroup, type FilterRow } from '../filter';

type ExpressionTemplate = (path: string, quoted: string, raw: string) => string;

/** Generates the JS expression equivalent of visual filter rows (Visual → Expression switch). */
export class FilterExpressionBuilder {
  static readonly matchAll = 'true';

  private static readonly templates: Partial<Record<FilterOp, ExpressionTemplate>> = {
    [FilterOp.Equals]: (p, q) => `${p} === ${q}`,
    [FilterOp.NotEquals]: (p, q) => `${p} !== ${q}`,
    [FilterOp.Contains]: (p, q) => `${p}.includes(${q})`,
    [FilterOp.StartsWith]: (p, q) => `${p}.startsWith(${q})`,
    [FilterOp.InList]: (p, _q, raw) => `${JSON.stringify(raw.split(',').map((s) => s.trim()))}.includes(${p})`,
    [FilterOp.Exists]: (p) => `${p} !== undefined`,
    [FilterOp.NumEquals]: (p, _q, raw) => `${p} === ${raw}`,
    [FilterOp.NumNotEquals]: (p, _q, raw) => `${p} !== ${raw}`,
  };

  static build(filter: FilterGroup): string {
    if (FilterGroups.isEmpty(filter)) return FilterExpressionBuilder.matchAll;
    return FilterExpressionBuilder.group(filter);
  }

  /** Rows and non-empty sub-groups joined by the combinator; sub-groups with several parts get parentheses. */
  static group(group: FilterGroup): string {
    const joiner = group.combinator === FilterCombinator.And ? ' && ' : ' || ';
    const nested = FilterGroups.children(group)
      .filter((g) => !FilterGroups.isEmpty(g))
      .map((g) => (FilterGroups.conditionCount(g) > 1 ? `(${FilterExpressionBuilder.group(g)})` : FilterExpressionBuilder.group(g)));
    return [...group.rows.map((row) => FilterExpressionBuilder.row(row)), ...nested].join(joiner);
  }

  static row(row: FilterRow): string {
    const path = `event.${row.field}`;
    const template = FilterExpressionBuilder.templates[row.op];
    const quoted = row.literal ? row.value : JSON.stringify(row.value);
    return template ? template(path, quoted, row.value) : `${path} ${row.op} ${row.value}`;
  }
}
