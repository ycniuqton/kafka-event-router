import { lazy, Suspense } from 'react';
import { TextArea } from '../../../../components/ui';
import type { CodeEditorProps } from './CodeEditorHandle';
import styles from './CodeEditor.module.css';

const MonacoCodeEditor = lazy(() => import('./MonacoCodeEditor'));

/** JS step editor: Monaco once its chunk has loaded, a plain monospace textarea meanwhile. */
export function JsCodeEditor(props: CodeEditorProps) {
  return (
    <Suspense fallback={<PlainCodeEditor {...props} />}>
      <MonacoCodeEditor {...props} />
    </Suspense>
  );
}

function PlainCodeEditor({ value, onChange, ariaLabel }: CodeEditorProps) {
  return <TextArea mono className={styles.fallback} spellCheck={false} value={value} aria-label={ariaLabel} onChange={(e) => onChange(e.target.value)} />;
}
