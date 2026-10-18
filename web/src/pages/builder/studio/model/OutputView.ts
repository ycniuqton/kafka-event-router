import { LineHighlight, type LineDecoration } from '../../../../components/ui';
import type { JsonLine, SampleEvent, TransformRunResult, TransformStep } from '../../../../domain';
import { EventSize } from './EventSize';
import { OutputDiff, type RemovedField } from './OutputDiff';
import { OutputMark } from './StudioEnums';
import { StudioText } from './StudioText';

/** Display values of the Output column (design `outLines`, `removed`, `outErr*`, key/headers/size). */
export class OutputView {
  static readonly markText: Record<OutputMark, string> = { [OutputMark.None]: '', [OutputMark.Added]: '+', [OutputMark.Changed]: '~' };
  static readonly markHighlight: Record<OutputMark, LineHighlight> = {
    [OutputMark.None]: LineHighlight.None,
    [OutputMark.Added]: LineHighlight.Added,
    [OutputMark.Changed]: LineHighlight.Changed,
  };

  private readonly input: SampleEvent;
  private readonly result: TransformRunResult;
  private readonly diff: OutputDiff | null;

  constructor(input: SampleEvent, result: TransformRunResult, diffOn: boolean) {
    this.input = input;
    this.result = result;
    this.diff = diffOn ? new OutputDiff(input.value, this.value) : null;
  }

  get value(): unknown {
    return this.result.event?.value ?? null;
  }

  get removed(): RemovedField[] {
    return this.value && this.diff ? this.diff.removed() : [];
  }

  decorate(line: JsonLine): LineDecoration | undefined {
    if (!this.diff) return undefined;
    const mark = this.diff.markOf(line);
    return { highlight: OutputView.markHighlight[mark], mark: OutputView.markText[mark] };
  }

  errorMessage(): string | null {
    return this.result.error === undefined ? null : StudioText.eventError(this.input.id, this.result.error);
  }

  errorStep(steps: TransformStep[]): string {
    const index = this.result.stepIndex ?? 0;
    return steps[index] ? StudioText.failedStep(index, steps[index].kind) : '';
  }

  get dropped(): boolean {
    return !!this.result.dropped;
  }

  get key(): string {
    return this.result.event?.key ?? StudioText.noValue;
  }

  get headers(): string {
    return this.result.event ? StudioText.headers(this.result.event.headers ?? {}) : StudioText.noValue;
  }

  get size(): string {
    return StudioText.size(EventSize.bytes(this.input.value), EventSize.bytes(this.value));
  }
}
