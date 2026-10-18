/** JSON value kinds shown as type icons in the Input column. */
export const JsonKind = {
  String: 'string',
  Number: 'number',
  Boolean: 'boolean',
  Null: 'null',
  Object: 'object',
  Array: 'array',
} as const;
export type JsonKind = (typeof JsonKind)[keyof typeof JsonKind];

/** Classifies a value and gives its icon and label. */
export class JsonKindInfo {
  static readonly icons: Record<JsonKind, string> = {
    [JsonKind.String]: 'Aa',
    [JsonKind.Number]: '#',
    [JsonKind.Boolean]: '◐',
    [JsonKind.Null]: '∅',
    [JsonKind.Object]: '{}',
    [JsonKind.Array]: '[]',
  };

  static of(value: unknown): JsonKind {
    if (value === null || value === undefined) return JsonKind.Null;
    if (Array.isArray(value)) return JsonKind.Array;
    const kind = typeof value;
    if (kind === 'number') return JsonKind.Number;
    if (kind === 'boolean') return JsonKind.Boolean;
    return kind === 'object' ? JsonKind.Object : JsonKind.String;
  }
}

/** Kind of a flattened JSON line (branches carry a `[n]` or `{n}` summary instead of a value). */
export class JsonLineKind {
  static readonly arrayPrefix = '[';

  static of(line: { isBranch: boolean; summary?: string; value?: unknown }): JsonKind {
    if (!line.isBranch) return JsonKindInfo.of(line.value);
    return line.summary?.startsWith(JsonLineKind.arrayPrefix) ? JsonKind.Array : JsonKind.Object;
  }
}
