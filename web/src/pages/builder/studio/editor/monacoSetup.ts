import { loader } from '@monaco-editor/react';
import * as monaco from 'monaco-editor';
import EditorWorker from 'monaco-editor/editor/editor.worker?worker';
import TsWorker from 'monaco-editor/language/typescript/ts.worker?worker';

/** Serves Monaco's web workers from the bundle (no CDN) and hands the local build to the React wrapper. */
class MonacoWorkers {
  static readonly scriptLanguages: ReadonlySet<string> = new Set(['typescript', 'javascript']);

  static install(): void {
    self.MonacoEnvironment = {
      getWorker: (_id: string, label: string) => (MonacoWorkers.scriptLanguages.has(label) ? new TsWorker() : new EditorWorker()),
    };
    loader.config({ monaco });
  }
}

MonacoWorkers.install();

export { monaco };
