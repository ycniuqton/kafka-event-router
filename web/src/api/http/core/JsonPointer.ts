/** RFC 6901 JSON Pointer: the path format of every problem `path` and `errors[].path`. */
export class JsonPointer {
  static readonly root = '';
  static readonly separator = '/';
  static readonly fieldSeparator = '.';
  private static readonly escapes: readonly (readonly [string, string])[] = [
    ['~1', '/'],
    ['~0', '~'],
  ];

  /** Splits a pointer into unescaped reference tokens (`/a~1b/0` → `['a/b', '0']`). */
  static parse(pointer: string): string[] {
    if (pointer === JsonPointer.root) return [];
    const body = pointer.startsWith(JsonPointer.separator) ? pointer.slice(1) : pointer;
    return body.split(JsonPointer.separator).map((token) => JsonPointer.unescape(token));
  }

  /** Builds a pointer from reference tokens. */
  static format(tokens: readonly (string | number)[]): string {
    return tokens.map((t) => JsonPointer.separator + JsonPointer.escape(String(t))).join('');
  }

  /** Dotted form-field name for a pointer (`/bootstrapServers/0` → `bootstrapServers.0`). */
  static toFieldName(pointer: string): string {
    return JsonPointer.parse(pointer).join(JsonPointer.fieldSeparator);
  }

  /** True when `pointer` equals `prefix` or lies below it. */
  static isWithin(pointer: string, prefix: string): boolean {
    return pointer === prefix || pointer.startsWith(prefix + JsonPointer.separator);
  }

  private static unescape(token: string): string {
    return JsonPointer.escapes.reduce((acc, [from, to]) => acc.split(from).join(to), token);
  }

  private static escape(token: string): string {
    return token.split('~').join('~0').split('/').join('~1');
  }
}
