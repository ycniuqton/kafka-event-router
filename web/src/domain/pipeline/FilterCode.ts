/**
 * Filter Code mode (spec §7.2, P2): a full JS function `function filter(event, ctx) { … return bool }`.
 * The engine's `expr` node wraps code as `return ( <code> )`, so the function is stored as an
 * immediately-invoked expression on one line prefix (error lines stay aligned).
 */
export class FilterCode {
  static readonly callSuffix = ')(event, ctx)';
  static readonly callPrefix = '(';
  static readonly template = 'function filter(event, ctx) {\n  return true;\n}';
  private static readonly wrapped = /^\(\s*(function[\s\S]*\})\s*\)\(event, ctx\)\s*$/;
  private static readonly singleReturn = /^function\s+\w*\s*\(event(?:,\s*ctx)?\)\s*\{\s*return\s+([\s\S]+?);?\s*\}$/;

  /** Function source → `expr` code. */
  static wrap(fn: string): string {
    return `${FilterCode.callPrefix}${fn.trim()}${FilterCode.callSuffix}`;
  }

  /** `expr` code → function source, or null when the code is a plain expression. */
  static unwrap(code: string): string | null {
    const match = FilterCode.wrapped.exec(code.trim());
    return match ? match[1] : null;
  }

  /** A function returning the expression (Expression → Code). */
  static fromExpression(expression: string): string {
    const body = expression.trim() || 'true';
    return `function filter(event, ctx) {\n  return ${body};\n}`;
  }

  /** The returned expression when the function is a single `return …;`, else null (Code → Expression). */
  static toExpression(fn: string): string | null {
    const match = FilterCode.singleReturn.exec(fn.trim());
    return match ? match[1].trim() : null;
  }
}
