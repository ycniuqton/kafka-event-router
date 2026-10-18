import { ExpressionParser, FilterCode, FilterExpressionBuilder, FilterGroups, FilterMode, type FilterGroup } from '../../../domain';

/**
 * Visual ⇄ Expression ⇄ Code switching: rows always convert to an expression or a function; an expression
 * always becomes a function; back only when the function is a single `return` / the parser can read it.
 */
export class FilterModeRules {
  static readonly options: { value: FilterMode; label: string }[] = [
    { value: FilterMode.Visual, label: 'Visual' },
    { value: FilterMode.Expression, label: 'Expression' },
    { value: FilterMode.Code, label: 'Code' },
  ];

  /** The patch that switches filter to mode, or null when that conversion is not possible. */
  static switchTo(filter: FilterGroup, mode: FilterMode): Partial<FilterGroup> | null {
    if (mode === FilterMode.Expression) return FilterModeRules.toExpression(filter);
    if (mode === FilterMode.Code) return FilterModeRules.toCode(filter);
    return FilterModeRules.toVisual(filter);
  }

  static toExpression(filter: FilterGroup): Partial<FilterGroup> | null {
    const expression = FilterModeRules.expressionOf(filter);
    return expression === null ? null : { mode: FilterMode.Expression, expression };
  }

  static toCode(filter: FilterGroup): Partial<FilterGroup> {
    const expression = filter.mode === FilterMode.Expression ? filter.expression : FilterExpressionBuilder.build(filter);
    return {
      mode: FilterMode.Code,
      expression: FilterCode.fromExpression(expression),
    };
  }

  /** The visual group for the current expression or function, or null when it is too complex to show as rows. */
  static toVisual(filter: FilterGroup): Partial<FilterGroup> | null {
    const expression = FilterModeRules.expressionOf(filter);
    const group = expression === null ? null : ExpressionParser.toGroup(expression);
    if (!group) return null;
    return {
      mode: FilterMode.Visual,
      combinator: group.combinator,
      rows: group.rows,
      groups: FilterGroups.children(group),
    };
  }

  /** The filter as one boolean expression, or null for a function with more than a single `return`. */
  private static expressionOf(filter: FilterGroup): string | null {
    if (filter.mode === FilterMode.Code) return FilterCode.toExpression(filter.expression);
    if (filter.mode === FilterMode.Expression) return filter.expression;
    return FilterExpressionBuilder.build(filter);
  }
}
