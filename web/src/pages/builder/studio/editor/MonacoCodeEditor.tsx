import Editor, { type OnMount } from '@monaco-editor/react';
import { useEffect, useMemo, useRef } from 'react';
import type * as Monaco from 'monaco-editor';
import { services } from '../../../../api';
import { useSettings } from '../../../../store';
import { useStudio } from '../hooks/StudioContext';
import { useSandboxInfo } from '../hooks/useSandboxInfo';
import type { CodeEditorProps, CodeMarker } from './CodeEditorHandle';
import { MonacoFieldCompletion } from './MonacoFieldCompletion';
import { MonacoSandboxCheck } from './MonacoSandboxCheck';
import { MonacoSandboxTypings } from './MonacoSandboxTypings';
import { MonacoThemes } from './MonacoThemes';
import { monaco } from './monacoSetup';
import styles from './CodeEditor.module.css';

/** Editor options matching the design's 12px/1.6 Geist Mono code box. */
class MonacoOptions {
  static readonly value: Monaco.editor.IStandaloneEditorConstructionOptions = {
    fontFamily: '"Geist Mono", ui-monospace, monospace',
    fontSize: 12,
    lineHeight: 19,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    automaticLayout: true,
    tabSize: 2,
    lineNumbersMinChars: 3,
    padding: { top: 8, bottom: 8 },
    renderLineHighlight: 'line',
    fixedOverflowWidgets: true,
  };
  static readonly language = 'javascript';
}

/** Puts run errors on the code as Monaco error markers (squiggle at line/column). */
class MonacoErrorMarkers {
  static readonly owner = 'transform-studio-run';

  static apply(editor: Monaco.editor.IStandaloneCodeEditor | null, markers: CodeMarker[] | undefined): void {
    const model = editor?.getModel();
    if (!model) return;
    const list = (markers ?? []).map((m) => ({
      severity: monaco.MarkerSeverity.Error,
      message: m.message,
      startLineNumber: m.line,
      startColumn: m.column,
      endLineNumber: m.line,
      endColumn: model.getLineMaxColumn(Math.min(m.line, model.getLineCount())),
    }));
    monaco.editor.setModelMarkers(model, MonacoErrorMarkers.owner, list);
  }
}

/** Monaco editor of a JavaScript step (lazy-loaded chunk) with sample-aware autocomplete. */
export default function MonacoCodeEditor({ value, markers, onChange, onReady, ariaLabel, entry = 'transform' }: CodeEditorProps) {
  const { theme } = useSettings();
  const inputLines = useStudio()?.model.inputLines;
  const editorRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null);
  const { info } = useSandboxInfo();
  const sandboxCheck = useMemo(() => new MonacoSandboxCheck(monaco, (code, kind) => services.sandbox.check(code, kind)), []);
  useEffect(() => () => sandboxCheck.dispose(), [sandboxCheck]);
  useEffect(() => sandboxCheck.schedule(editorRef.current?.getModel(), entry), [sandboxCheck, value, entry]);
  useEffect(() => {
    if (inputLines) MonacoFieldCompletion.update(inputLines);
  }, [inputLines]);
  useEffect(() => MonacoSandboxTypings.apply(monaco, info?.typings), [info?.typings]);
  useEffect(() => MonacoThemes.apply(monaco, theme), [theme]);
  useEffect(() => () => onReady?.(null), [onReady]);
  useEffect(() => MonacoErrorMarkers.apply(editorRef.current, markers), [markers, value]);
  const onMount: OnMount = (editor) => {
    editorRef.current = editor;
    MonacoFieldCompletion.register(monaco);
    MonacoThemes.apply(monaco, theme);
    MonacoErrorMarkers.apply(editor, markers);
    sandboxCheck.schedule(editor.getModel(), entry);
    onReady?.({
      insert: (text) => {
        const selection = editor.getSelection();
        if (selection) editor.executeEdits('snippet', [{ range: selection, text, forceMoveMarkers: true }]);
        editor.focus();
      },
    });
  };
  return (
    <div className={styles.frame}>
      <Editor
        height="100%"
        language={MonacoOptions.language}
        value={value}
        theme={MonacoThemes.names[theme]}
        options={{ ...MonacoOptions.value, ariaLabel }}
        onChange={(next) => onChange(next ?? '')}
        onMount={onMount}
      />
    </div>
  );
}
