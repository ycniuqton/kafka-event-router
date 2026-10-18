/** One rendered line of a JSON tree: either an object/array header or a leaf value. */
export type JsonLine = {
  path: string;
  key: string;
  depth: number;
  isBranch: boolean;
  summary?: string;
  value?: unknown;
};

/** Flattens JSON into display lines (design `flatten`), used by JsonTree and diffs. */
export class JsonFlattener {
  static flatten(node: unknown, prefix = '', depth = 0, out: JsonLine[] = []): JsonLine[] {
    const source = node as Record<string, unknown>;
    const isArray = Array.isArray(node);
    for (const key of Object.keys(source)) {
      const value = source[key];
      const path = prefix ? `${prefix}.${key}` : key;
      const label = isArray ? `[${key}]` : key;
      if (JsonFlattener.isBranch(value)) {
        out.push({ path, key: label, depth, isBranch: true, summary: JsonFlattener.summary(value) });
        JsonFlattener.flatten(value, path, depth + 1, out);
      } else {
        out.push({ path, key: label, depth, isBranch: false, value });
      }
    }
    return out;
  }

  /** Map of path → serialised leaf, used to diff input and output. */
  static leafMap(node: unknown): Record<string, string> {
    const map: Record<string, string> = {};
    for (const line of JsonFlattener.flatten(node)) {
      map[line.path] = line.isBranch ? JsonFlattener.branchMarker : JSON.stringify(line.value);
    }
    return map;
  }

  static readonly branchMarker = '__obj';

  static isBranch(value: unknown): value is object {
    return !!value && typeof value === 'object' && Object.keys(value).length > 0;
  }

  /** True for the line of a non-empty array (its summary is `[n]`). */
  static isArray(line: JsonLine): boolean {
    return line.isBranch && !!line.summary?.startsWith(JsonFlattener.arrayOpen);
  }

  static readonly arrayOpen = '[';

  static summary(value: object): string {
    return Array.isArray(value) ? `[${value.length}]` : `{${Object.keys(value).length}}`;
  }
}
