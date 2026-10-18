/** Inserts a snippet at the start of the `transform` body when no editor cursor is available. */
export class SnippetInserter {
  static readonly functionHead = /function\s+transform\s*\([^)]*\)\s*\{[^\n]*\n/;

  static insert(code: string, snippet: string): string {
    const head = SnippetInserter.functionHead.exec(code);
    if (!head) return `${code}\n${snippet}`;
    const at = head.index + head[0].length;
    return code.slice(0, at) + snippet + code.slice(at);
  }
}
