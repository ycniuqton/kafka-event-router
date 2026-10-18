import type { Diagnostic, SandboxEntry } from '../../domain';

/** Browser stand-in for the sandbox compile check: parses code without running it (mock mode only). */
export class MockCodeChecker {
  static readonly transformMissing = 'transform(event, ctx) is not defined';
  static readonly emptyExpression = 'expression is empty';
  static readonly transformDeclaration = /\bfunction\s+transform\s*\(|\b(?:const|let|var)\s+transform\s*=/;

  static check(code: string, entry: SandboxEntry): Diagnostic[] {
    return entry === 'predicate' ? MockCodeChecker.predicate(code) : MockCodeChecker.transform(code);
  }

  private static transform(code: string): Diagnostic[] {
    const syntax = MockCodeChecker.parse(() => new Function(code));
    if (syntax) return [syntax];
    return MockCodeChecker.transformDeclaration.test(code) ? [] : [MockCodeChecker.error(MockCodeChecker.transformMissing)];
  }

  private static predicate(code: string): Diagnostic[] {
    if (!code.trim()) return [MockCodeChecker.error(MockCodeChecker.emptyExpression)];
    const syntax = MockCodeChecker.parse(() => new Function('event', 'ctx', `return (${code}\n);`));
    return syntax ? [syntax] : [];
  }

  private static parse(compile: () => unknown): Diagnostic | null {
    try {
      compile();
      return null;
    } catch (e) {
      return MockCodeChecker.error(`${(e as Error).name}: ${(e as Error).message}`);
    }
  }

  private static error(message: string): Diagnostic {
    return { severity: 'error', message, line: null, column: null, path: null };
  }
}
