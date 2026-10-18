import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type * as Monaco from 'monaco-editor';
import type { Diagnostic } from '../../../../domain';
import { MonacoSandboxCheck } from './MonacoSandboxCheck';

class FakeModel {
  value: string;
  disposed = false;
  constructor(value: string) {
    this.value = value;
  }
  getValue = () => this.value;
  isDisposed = () => this.disposed;
  getLineCount = () => this.value.split('\n').length;
  getLineMaxColumn = (line: number) => this.value.split('\n')[line - 1].length + 1;
}

function setup(answer: (code: string) => Diagnostic[] | Promise<Diagnostic[]>) {
  const setModelMarkers = vi.fn();
  const monaco = { editor: { setModelMarkers }, MarkerSeverity: { Error: 8, Warning: 4 } } as unknown as typeof Monaco;
  const check = vi.fn(async (code: string) => answer(code));
  return { setModelMarkers, check, sandbox: new MonacoSandboxCheck(monaco, check) };
}

const asModel = (m: FakeModel) => m as unknown as Monaco.editor.ITextModel;
const syntax: Diagnostic = { severity: 'error', message: 'SyntaxError', line: 2, column: 3, path: null };

describe('MonacoSandboxCheck', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('debounces typing into one check and marks the reported position', async () => {
    const { sandbox, check, setModelMarkers } = setup(() => [syntax]);
    const model = new FakeModel('function transform(e) {\n  let x = ;\n}');
    sandbox.schedule(asModel(model), 'transform');
    sandbox.schedule(asModel(model), 'transform');
    await vi.advanceTimersByTimeAsync(MonacoSandboxCheck.debounceMs);
    expect(check).toHaveBeenCalledTimes(1);
    const [, owner, markers] = setModelMarkers.mock.calls[0];
    expect(owner).toBe(MonacoSandboxCheck.owner);
    expect(markers[0]).toMatchObject({ severity: 8, startLineNumber: 2, startColumn: 3, endColumn: 12 });
  });

  it('drops answers for code that changed and clears markers when the check fails', async () => {
    const model = new FakeModel('a');
    const { sandbox, setModelMarkers } = setup(() => {
      model.value = 'ab';
      return [syntax];
    });
    sandbox.schedule(asModel(model), 'transform');
    await vi.advanceTimersByTimeAsync(MonacoSandboxCheck.debounceMs);
    expect(setModelMarkers).not.toHaveBeenCalled();
    const failing = setup(() => Promise.reject(new Error('offline')));
    failing.sandbox.schedule(asModel(new FakeModel('x')), 'predicate');
    await vi.advanceTimersByTimeAsync(MonacoSandboxCheck.debounceMs);
    expect(failing.setModelMarkers.mock.calls[0][2]).toEqual([]);
  });

  it('marks the first line when a diagnostic has no position and stops after dispose', async () => {
    const { sandbox, setModelMarkers, check } = setup(() => [{ ...syntax, line: null, column: null, severity: 'warning' }]);
    const model = new FakeModel('function nope() {}');
    sandbox.schedule(asModel(model), 'transform');
    await vi.advanceTimersByTimeAsync(MonacoSandboxCheck.debounceMs);
    expect(setModelMarkers.mock.calls[0][2][0]).toMatchObject({ severity: 4, startLineNumber: 1, startColumn: 1 });
    sandbox.schedule(asModel(model), 'transform');
    sandbox.dispose();
    await vi.advanceTimersByTimeAsync(MonacoSandboxCheck.debounceMs);
    expect(check).toHaveBeenCalledTimes(1);
  });
});
