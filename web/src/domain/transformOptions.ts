/** Options of recipe steps beyond a field path: value sources, type conversion and constants. */

/** Where set / set-key / mapper fields read their value (engine `valueSource.from`). */
export const ValueSourceKind = { Constant: 'constant', Field: 'field', Metadata: 'metadata', Formula: 'formula' } as const;
export type ValueSourceKind = (typeof ValueSourceKind)[keyof typeof ValueSourceKind];

/** Record metadata a value source can read (engine `metadataName`). */
export const MetadataName = {
  Topic: 'topic',
  Partition: 'partition',
  Offset: 'offset',
  Timestamp: 'timestamp',
  RouteName: 'route_name',
  RouteVersion: 'route_version',
} as const;
export type MetadataName = (typeof MetadataName)[keyof typeof MetadataName];

/** Target types of the convert-type recipe. */
export const ConvertTarget = { String: 'string', Number: 'number', Integer: 'integer', Boolean: 'boolean' } as const;
export type ConvertTarget = (typeof ConvertTarget)[keyof typeof ConvertTarget];

/** What convert-type does with a value it cannot convert. */
export const ConvertFailure = { Error: 'error', Null: 'null', Keep: 'keep' } as const;
export type ConvertFailure = (typeof ConvertFailure)[keyof typeof ConvertFailure];

export const ValueSourceLabel: Record<ValueSourceKind, string> = {
  [ValueSourceKind.Constant]: 'Fixed value',
  [ValueSourceKind.Field]: 'From field',
  [ValueSourceKind.Metadata]: 'From metadata',
  [ValueSourceKind.Formula]: 'Formula',
};

export const ConvertTargetLabel: Record<ConvertTarget, string> = {
  [ConvertTarget.String]: 'Text',
  [ConvertTarget.Number]: 'Number',
  [ConvertTarget.Integer]: 'Integer',
  [ConvertTarget.Boolean]: 'Boolean',
};

export const ConvertFailureLabel: Record<ConvertFailure, string> = {
  [ConvertFailure.Error]: 'Fail the event',
  [ConvertFailure.Null]: 'Set null',
  [ConvertFailure.Keep]: 'Keep as is',
};

/**
 * Typed constant of a "Fixed value": JSON literals (numbers, true/false, null, quoted strings, objects)
 * keep their type, any other text is a string.
 */
export class StepConstant {
  static parse(text: string): unknown {
    const trimmed = text.trim();
    if (!trimmed) return text;
    try {
      return JSON.parse(trimmed) as unknown;
    } catch {
      return text;
    }
  }

  static format(value: unknown): string {
    return typeof value === 'string' && !StepConstant.looksLiteral(value) ? value : JSON.stringify(value);
  }

  private static looksLiteral(text: string): boolean {
    return StepConstant.parse(text) !== text;
  }
}

/** Field list of remove / keep-only steps: `fields` when set, else the single `field`. */
export class StepFieldList {
  static readonly separator = ',';

  static of(step: { field: string; fields?: string[] }): string[] {
    if (step.fields && step.fields.length) return step.fields;
    return step.field ? [step.field] : [];
  }

  static parse(text: string): string[] {
    return text
      .split(StepFieldList.separator)
      .map((p) => p.trim())
      .filter(Boolean);
  }

  static format(fields: string[]): string {
    return fields.join(`${StepFieldList.separator} `);
  }
}
