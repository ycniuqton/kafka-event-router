import { describe, expect, it } from 'vitest';
import { FilterCombinator, FilterMode, FilterOp, type FilterGroup } from '../filter';
import { ExpressionParser } from './ExpressionParser';
import { FilterEvaluator } from './FilterEvaluator';
import { FilterExpressionBuilder } from './FilterExpressionBuilder';

const group = (combinator: FilterCombinator, rows: FilterGroup['rows'], groups: FilterGroup[] = []): FilterGroup => ({
  combinator,
  rows,
  mode: FilterMode.Visual,
  expression: '',
  groups,
});

describe('ExpressionParser', () => {
  it('reads every operator the expression builder emits', () => {
    const rows: FilterGroup['rows'] = [
      { field: 'value.type', op: FilterOp.Equals, value: 'OrderPaid' },
      { field: 'value.type', op: FilterOp.NotEquals, value: 'x"y' },
      { field: 'value.name', op: FilterOp.Contains, value: 'ab' },
      { field: 'value.name', op: FilterOp.StartsWith, value: 'a' },
      { field: 'value.country', op: FilterOp.InList, value: 'DE, FR' },
      { field: 'value.email', op: FilterOp.Exists, value: '' },
      { field: 'value.amount', op: FilterOp.NumEquals, value: '5' },
      { field: 'value.amount', op: FilterOp.NumNotEquals, value: '6' },
      { field: 'value.amount', op: FilterOp.GreaterThan, value: '100' },
      { field: 'value.amount', op: FilterOp.LessThan, value: '-2.5' },
      { field: 'value.keep', op: FilterOp.Equals, value: 'true', literal: true },
    ];
    const source = group(FilterCombinator.And, rows);
    const parsed = ExpressionParser.toGroup(FilterExpressionBuilder.build(source));
    expect(parsed?.rows).toEqual(rows);
    expect(parsed?.combinator).toBe(FilterCombinator.And);
  });

  it('round-trips nested groups with parentheses and precedence', () => {
    const nested = group(FilterCombinator.And, [{ field: 'value.a', op: FilterOp.Equals, value: '1' }], [
      group(FilterCombinator.Or, [
        { field: 'value.b', op: FilterOp.Equals, value: '2' },
        { field: 'value.c', op: FilterOp.GreaterThan, value: '3' },
      ]),
    ]);
    const expression = FilterExpressionBuilder.build(nested);
    expect(expression).toBe('event.value.a === "1" && (event.value.b === "2" || event.value.c > 3)');
    expect(ExpressionParser.toGroup(expression)).toEqual(nested);
    const precedence = ExpressionParser.toGroup('event.value.a === "1" && event.value.b === "2" || event.value.c === "3"');
    expect(precedence?.combinator).toBe(FilterCombinator.Or);
    expect(precedence?.groups?.[0].rows.map((r) => r.field)).toEqual(['value.a', 'value.b']);
  });

  it('returns null for expressions the builder cannot show', () => {
    for (const code of ['event.value.a >= 1', '!event.value.a', 'event.value.a.toLowerCase() === "x"', 'ctx.now > 1', 'event.value.a === other', 'event.value.a === "x" &&']) {
      expect(ExpressionParser.toGroup(code)).toBeNull();
    }
    expect(ExpressionParser.toGroup('true')?.rows).toEqual([]);
  });

  it('evaluates nested groups and literals like the engine', () => {
    const f = ExpressionParser.toGroup('event.value.keep === true && (event.value.n > 2 || event.value.t === "x")') as FilterGroup;
    expect(FilterEvaluator.evaluate({ value: { keep: true, n: 3 } }, f).pass).toBe(true);
    expect(FilterEvaluator.evaluate({ value: { keep: 'true', n: 3 } }, f).pass).toBe(false);
    expect(FilterEvaluator.evaluate({ value: { keep: true, n: 1, t: 'y' } }, f).pass).toBe(false);
  });
});
