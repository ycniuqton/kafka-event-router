import { useMemo } from 'react';
import { Button, ButtonSize, ButtonVariant } from '../../../../../components/ui';
import { useStudio } from '../../hooks/StudioContext';
import { StepConsole } from '../../model/StepConsole';
import { StudioText } from '../../model/StudioText';
import styles from './JsConsole.module.css';

/** Console tab: ctx.log() output of every sample event up to this step. */
export function JsConsole({ index }: { index: number }) {
  const { model, commands } = useStudio();
  const run = model.preview.run;
  const entries = useMemo(() => StepConsole.collect(model.samples, run, index), [model.samples, run, index]);
  if (!entries.length) {
    return <div className={styles.empty}>No ctx.log() output. Call ctx.log(…) in your code to see values per event here.</div>;
  }
  return (
    <div className={styles.console}>
      {entries.map((entry) => (
        <div key={entry.sampleId} className={styles.entry} data-current={entry.sampleId === model.current?.id}>
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} className={styles.event} onClick={() => commands.selectSample(entry.sampleId)}>
            #{StudioText.eventNumber(entry.sampleId)}
          </Button>
          <div className={styles.lines}>
            {entry.lines.map((line, i) => (
              <div key={i}>{line}</div>
            ))}
            {entry.error && <div className={styles.error}>{entry.error}</div>}
          </div>
        </div>
      ))}
    </div>
  );
}
