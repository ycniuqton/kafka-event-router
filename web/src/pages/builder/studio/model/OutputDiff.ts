import { FieldPath, JsonFlattener, type JsonLine } from '../../../../domain';
import { OutputMark } from './StudioEnums';

/** An input field missing from the output, listed struck-through under the diff. */
export type RemovedField = { path: string; value: string };

/** Compares input and output payloads: added (green), changed (amber), removed (red). */
export class OutputDiff {
  private readonly before: Record<string, string>;
  private readonly after: Record<string, string>;
  private readonly inputLines: JsonLine[];

  constructor(input: unknown, output: unknown) {
    this.inputLines = JsonFlattener.flatten(input);
    this.before = JsonFlattener.leafMap(input);
    this.after = output ? JsonFlattener.leafMap(output) : {};
  }

  markOf(line: JsonLine): OutputMark {
    const was = this.before[line.path];
    if (was === undefined) return OutputMark.Added;
    if (was !== this.after[line.path] && !line.isBranch) return OutputMark.Changed;
    return OutputMark.None;
  }

  /** Top-most removed input fields (children of a removed object are not repeated). */
  removed(): RemovedField[] {
    return this.inputLines
      .filter((l) => this.after[l.path] === undefined && this.parentKept(l.path))
      .map((l) => ({ path: l.path, value: l.isBranch ? (l.summary ?? '') : JSON.stringify(l.value) }));
  }

  private parentKept(path: string): boolean {
    const cut = path.lastIndexOf(FieldPath.separator);
    return cut < 0 || this.after[path.slice(0, cut)] !== undefined;
  }
}
