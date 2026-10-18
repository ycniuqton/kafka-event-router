import { Button } from '../../../components/ui';
import styles from './FormFooter.module.css';

export type FormFooterProps = {
  step: number;
  lastStep: number;
  saving: boolean;
  testing: boolean;
  onBack: () => void;
  onNext: () => void;
  onTest: () => void;
  onSave: () => void;
};

/** Back · Test · Next, or Save & test on the last step of the connection panel. */
export function FormFooter({ step, lastStep, saving, testing, onBack, onNext, onTest, onSave }: FormFooterProps) {
  return (
    <>
      {step > 0 && (
        <Button size="lg" onClick={onBack}>
          Back
        </Button>
      )}
      <span className={styles.spacer} />
      <Button size="lg" disabled={testing || saving} onClick={onTest} title="Test these settings without saving">
        {testing ? 'Testing…' : 'Test'}
      </Button>
      {step < lastStep && (
        <Button size="lg" variant="primary" onClick={onNext}>
          Next
        </Button>
      )}
      {step === lastStep && (
        <Button size="lg" variant="primary" disabled={saving || testing} onClick={onSave}>
          Save &amp; test
        </Button>
      )}
    </>
  );
}
