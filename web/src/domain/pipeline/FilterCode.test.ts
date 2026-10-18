import { describe, expect, it } from 'vitest';
import { FilterCombinator, FilterMode, type FilterGroup } from '../filter';
import { FilterTreeModel } from '../routeConfig/FilterTreeModel';
import { FilterCode } from './FilterCode';
import { FilterEvaluator } from './FilterEvaluator';

const fn = 'function filter(event, ctx) {\n  const v = event.value;\n  return v.amount > 100 && !!ctx;\n}';
const group = (mode: FilterMode, expression: string): FilterGroup => ({
  combinator: FilterCombinator.And,
  rows: [],
  mode,
  expression,
  groups: [],
});

describe('FilterCode (Code mode, stored as an expr node)', () => {
  it('wraps a function as an immediately-invoked expr and reads it back as Code mode', () => {
    const node = FilterTreeModel.toNode(group(FilterMode.Code, fn));
    expect(node).toEqual({ type: 'expr', code: `(${fn})(event, ctx)` });
    const read = FilterTreeModel.fromNode(node);
    expect(read.group.mode).toBe(FilterMode.Code);
    expect(read.group.expression).toBe(fn);
  });

  it('keeps plain expressions in Expression mode', () => {
    const read = FilterTreeModel.fromNode({
      type: 'expr',
      code: 'event.value.amount > 1',
    });
    expect(read.group.mode).toBe(FilterMode.Expression);
    expect(FilterCode.unwrap('event.value.amount > 1')).toBeNull();
  });

  it('evaluates the function like the engine (event and ctx)', () => {
    expect(FilterEvaluator.evaluate({ value: { amount: 150 } }, group(FilterMode.Code, fn)).pass).toBe(true);
    expect(FilterEvaluator.evaluate({ value: { amount: 50 } }, group(FilterMode.Code, fn)).pass).toBe(false);
    expect(FilterEvaluator.evaluate({}, group(FilterMode.Code, 'function filter(event) { return event.value.x; }')).error).toBeTruthy();
  });

  it('converts between expression and a single-return function', () => {
    const code = FilterCode.fromExpression('event.value.amount > 100');
    expect(FilterCode.toExpression(code)).toBe('event.value.amount > 100');
    expect(FilterCode.toExpression(fn)).toBeNull();
    expect(FilterCode.fromExpression('')).toContain('return true;');
  });
});
