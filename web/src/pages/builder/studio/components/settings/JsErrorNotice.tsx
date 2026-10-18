import { Button, ButtonSize, ButtonVariant } from '../../../../../components/ui';
import { useStudio } from '../../hooks/StudioContext';
import type { SampleFailure } from '../../model/SampleRunReport';
import { StudioText } from '../../model/StudioText';
import styles from './JsErrorNotice.module.css';

/** Inline error of a JS step with the event that caused it and a jump to that event. */
export function JsErrorNotice({ failure }: { failure: SampleFailure }) {
  const { model, commands } = useStudio();
  const elsewhere = failure.sampleId !== model.current?.id;
  return (
    <div className={styles.error} role="alert">
      <span className={styles.message}>{StudioText.eventError(failure.sampleId, failure.message)}</span>
      {elsewhere && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} className={styles.jump} onClick={() => commands.selectSample(failure.sampleId)}>
          {StudioText.jumpToEvent(failure.sampleId)}
        </Button>
      )}
    </div>
  );
}
