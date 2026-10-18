import { TextArea } from '../../../../components/ui';
import styles from './ExpressionEditor.module.css';

export type ExpressionEditorProps = {
  value: string;
  error: string;
  onChange: (value: string) => void;
};

/** Expression mode: a JS boolean expression over `event`, with the first evaluation error shown. */
export function ExpressionEditor({ value, error, onChange }: ExpressionEditorProps) {
  return (
    <div className={styles.editor}>
      <TextArea mono sunken={false} spellCheck={false} className={styles.area} value={value} onChange={(e) => onChange(e.target.value)} />
      <div className={styles.help}>
        A JavaScript boolean expression. <span className={styles.mono}>event</span> has key, value, headers, topic, partition, offset, timestamp.
      </div>
      {error && <div className={styles.error}>{error}</div>}
    </div>
  );
}
