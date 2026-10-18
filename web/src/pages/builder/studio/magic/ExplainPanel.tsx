import { useEffect, useState } from 'react';
import { services, type StepExplanation } from '../../../../api';
import type { TransformStep } from '../../../../domain';
import { ExplainText } from './MagicText';
import styles from './Magic.module.css';


/** Plain-language summary of one JS step (LLM provider) or a recipe chain (rule-based), shown under the card. */
export function ExplainPanel({ steps }: { steps: TransformStep[] }) {
  const [text, setText] = useState<StepExplanation | null>(null);
  const [error, setError] = useState<string | null>(null);
  const key = JSON.stringify(steps);
  useEffect(() => {
    let live = true;
    services.assist.explain(JSON.parse(key) as TransformStep[]).then(
      (t) => live && setText(t),
      (e: Error) => live && setError(e.message),
    );
    return () => {
      live = false;
    };
  }, [key]);
  if (error) return <div className={styles.error} role="alert">{error}</div>;
  if (text === null) return <div className={styles.none}>{ExplainText.loading}</div>;
  return (
    <div className={styles.explanation} aria-label="Explanation">
      {text.summary}
      {text.bullets.length > 0 && (
        <ul className={styles.bullets}>
          {text.bullets.map((b, i) => (
            <li key={`${i}-${b}`}>{b}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
