/** RFC 6901 pointers as used by preview patches and filter node results. */
export class PatchPointer {
  static readonly root = '';
  static readonly separator = '/';
  private static readonly escapes: readonly (readonly [string, string])[] = [
    ['~1', '/'],
    ['~0', '~'],
  ];

  static parse(pointer: string): string[] {
    if (pointer === PatchPointer.root) return [];
    return pointer.slice(1).split(PatchPointer.separator).map((token) => PatchPointer.unescape(token));
  }

  static child(pointer: string, token: string | number): string {
    return pointer + PatchPointer.separator + PatchPointer.escape(String(token));
  }

  private static unescape(token: string): string {
    return PatchPointer.escapes.reduce((acc, [from, to]) => acc.split(from).join(to), token);
  }

  private static escape(token: string): string {
    return token.split('~').join('~0').split('/').join('~1');
  }
}
