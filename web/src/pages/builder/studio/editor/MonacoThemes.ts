import type * as Monaco from 'monaco-editor';
import { Theme } from '../../../../domain';

/** Monaco themes derived from the app's colour tokens, redefined when the app theme changes. */
export class MonacoThemes {
  static readonly names: Record<Theme, string> = { [Theme.Dark]: 'event-router-dark', [Theme.Light]: 'event-router-light' };
  static readonly bases: Record<Theme, Monaco.editor.BuiltinTheme> = { [Theme.Dark]: 'vs-dark', [Theme.Light]: 'vs' };
  static readonly tokens = { background: '--bg', foreground: '--tx', lineNumber: '--tx3', selection: '--p3', lineHighlight: '--p2' } as const;

  static apply(monaco: typeof Monaco, theme: Theme): void {
    const css = getComputedStyle(document.documentElement);
    const read = (token: string) => css.getPropertyValue(token).trim();
    const t = MonacoThemes.tokens;
    monaco.editor.defineTheme(MonacoThemes.names[theme], {
      base: MonacoThemes.bases[theme],
      inherit: true,
      rules: [],
      colors: {
        'editor.background': read(t.background),
        'editor.foreground': read(t.foreground),
        'editorLineNumber.foreground': read(t.lineNumber),
        'editor.selectionBackground': read(t.selection),
        'editor.lineHighlightBackground': read(t.lineHighlight),
      },
    });
    monaco.editor.setTheme(MonacoThemes.names[theme]);
  }
}
