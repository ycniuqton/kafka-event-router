import type { JsonLine } from '../../../../domain';

/** Field choices of a step's select: every input path, keeping a custom current value (design `fieldOpts`). */
export class FieldOptions {
  static of(current: string, lines: JsonLine[]): string[] {
    const paths = lines.map((l) => l.path);
    return !current || paths.includes(current) ? paths : [current, ...paths];
  }
}
