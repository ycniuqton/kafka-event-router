import { JsCodeEditor } from '../../studio/editor/JsCodeEditor';
import styles from './ExpressionEditor.module.css';

export type CodeFilterEditorProps = {
  value: string;
  error: string;
  onChange: (value: string) => void;
};

/** Code mode: a full JS function `filter(event, ctx)` in Monaco; the event is kept when it returns true. */
export function CodeFilterEditor({ value, error, onChange }: CodeFilterEditorProps) {
  return (
    <div className={styles.editor}>
      <JsCodeEditor value={value} onChange={onChange} entry="predicate" ariaLabel="Filter function" />
      <div className={styles.help}>
        A JavaScript function. Return <span className={styles.mono}>true</span> to keep the event; <span className={styles.mono}>event</span> has key, value,
        headers, topic, partition, offset, timestamp and <span className={styles.mono}>ctx</span> the sandbox helpers.
      </div>
      {error && <div className={styles.error}>{error}</div>}
    </div>
  );
}
