import { describe, expect, it } from 'vitest';
import { MockCodeChecker } from './MockCodeChecker';

describe('MockCodeChecker', () => {
  it('accepts a transform and reports syntax errors and a missing transform', () => {
    expect(MockCodeChecker.check('function transform(e, ctx) { return e; }', 'transform')).toEqual([]);
    expect(MockCodeChecker.check('const transform = (e) => e;', 'transform')).toEqual([]);
    expect(MockCodeChecker.check('function transform(e) { let x = ; }', 'transform')[0].message).toMatch(/SyntaxError/);
    expect(MockCodeChecker.check('function nope() {}', 'transform')[0].message).toBe(MockCodeChecker.transformMissing);
  });

  it('checks predicates as expressions without running them', () => {
    expect(MockCodeChecker.check('event.value.n > 1', 'predicate')).toEqual([]);
    expect(MockCodeChecker.check('event.value.n > 1 &&', 'predicate')[0].severity).toBe('error');
    expect(MockCodeChecker.check('  ', 'predicate')[0].message).toBe(MockCodeChecker.emptyExpression);
  });
});
