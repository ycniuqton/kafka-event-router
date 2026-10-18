import type * as Monaco from 'monaco-editor';
import { FieldPath, type JsonLine } from '../../../../domain';
import { JsonLineKind } from '../model/JsonKind';

/** Autocomplete of `event.value.` with the real fields of the current sample and their types. */
export class MonacoFieldCompletion {
  static readonly language = 'javascript';
  static readonly triggerCharacters = ['.'];
  static readonly valueAccess = /event\.value\.([\w.]*)$/;
  private static lines: JsonLine[] = [];
  private static registered = false;

  static update(lines: JsonLine[]): void {
    MonacoFieldCompletion.lines = lines;
  }

  static register(monaco: typeof Monaco): void {
    if (MonacoFieldCompletion.registered) return;
    MonacoFieldCompletion.registered = true;
    monaco.languages.registerCompletionItemProvider(MonacoFieldCompletion.language, {
      triggerCharacters: MonacoFieldCompletion.triggerCharacters,
      provideCompletionItems: (model, position) => {
        const before = model.getValueInRange({ startLineNumber: position.lineNumber, startColumn: 1, endLineNumber: position.lineNumber, endColumn: position.column });
        const match = MonacoFieldCompletion.valueAccess.exec(before);
        if (!match) return { suggestions: [] };
        return { suggestions: MonacoFieldCompletion.suggest(monaco, match[1], model.getWordUntilPosition(position), position.lineNumber) };
      },
    });
  }

  private static suggest(monaco: typeof Monaco, typed: string, word: Monaco.editor.IWordAtPosition, line: number): Monaco.languages.CompletionItem[] {
    const cut = typed.lastIndexOf(FieldPath.separator);
    const parent = cut < 0 ? '' : typed.slice(0, cut);
    const range = { startLineNumber: line, endLineNumber: line, startColumn: word.startColumn, endColumn: word.endColumn };
    return MonacoFieldCompletion.lines
      .filter((l) => MonacoFieldCompletion.parentOf(l.path) === parent)
      .map((l) => {
        const key = l.path.slice(parent ? parent.length + 1 : 0);
        return { label: key, kind: monaco.languages.CompletionItemKind.Field, detail: JsonLineKind.of(l), insertText: key, range };
      });
  }

  private static parentOf(path: string): string {
    const cut = path.lastIndexOf(FieldPath.separator);
    return cut < 0 ? '' : path.slice(0, cut);
  }
}
