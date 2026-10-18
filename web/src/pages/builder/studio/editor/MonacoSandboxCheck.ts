import type * as Monaco from 'monaco-editor';
import type { Diagnostic, SandboxEntry } from '../../../../domain';

/** Function that compiles code in the sandbox (`services.sandbox.check`). */
export type SandboxCheckFn = (code: string, entry: SandboxEntry) => Promise<Diagnostic[]>;

/** Debounced sandbox compile check: shows syntax and contract errors as squiggles while typing. */
export class MonacoSandboxCheck {
  static readonly owner = 'js-sandbox-check';
  static readonly debounceMs = 400;
  private readonly monaco: typeof Monaco;
  private readonly check: SandboxCheckFn;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private sequence = 0;

  constructor(monaco: typeof Monaco, check: SandboxCheckFn) {
    this.monaco = monaco;
    this.check = check;
  }

  /** Checks the model's code after the debounce; answers for outdated code are dropped. */
  schedule(model: Monaco.editor.ITextModel | null | undefined, entry: SandboxEntry): void {
    this.cancel();
    if (!model) return;
    this.timer = setTimeout(() => void this.run(model, entry), MonacoSandboxCheck.debounceMs);
  }

  dispose(): void {
    this.cancel();
    this.sequence++;
  }

  private cancel(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
  }

  private async run(model: Monaco.editor.ITextModel, entry: SandboxEntry): Promise<void> {
    const ticket = ++this.sequence;
    const code = model.getValue();
    const diagnostics = await this.check(code, entry).catch(() => [] as Diagnostic[]);
    if (ticket !== this.sequence || model.isDisposed() || model.getValue() !== code) return;
    this.monaco.editor.setModelMarkers(model, MonacoSandboxCheck.owner, MonacoSandboxCheck.markers(this.monaco, model, diagnostics));
  }

  /** Marker per diagnostic; without a position the whole first line is marked. */
  static markers(monaco: typeof Monaco, model: Monaco.editor.ITextModel, diagnostics: Diagnostic[]): Monaco.editor.IMarkerData[] {
    return diagnostics.map((d) => {
      const line = Math.min(Math.max(d.line ?? 1, 1), model.getLineCount());
      return {
        severity: d.severity === 'warning' ? monaco.MarkerSeverity.Warning : monaco.MarkerSeverity.Error,
        message: d.message,
        source: 'sandbox',
        startLineNumber: line,
        startColumn: d.column ?? 1,
        endLineNumber: line,
        endColumn: model.getLineMaxColumn(line),
      };
    });
  }
}
