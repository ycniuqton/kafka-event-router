import { Mono } from '../../../../components/ui';
import { LibraryRules, Tone, type TransformStep } from '../../../../domain';
import { LibraryPin } from './LibraryPin';
import { UpdateAvailableBadge } from './UpdateAvailableBadge';
import styles from './Library.module.css';

/** Card line of a library-pinned step: the pinned `name vN` and the update badge (null for other steps). */
export function LibraryStepCard({ step }: { step: TransformStep }) {
  const pin = LibraryPin.of(step);
  if (!pin) return null;
  return (
    <div className={styles.pin} data-testid="library-pin">
      <Mono tone={Tone.Accent}>{LibraryRules.pinLabel(pin.name, pin.version)}</Mono>
      <UpdateAvailableBadge step={step} />
    </div>
  );
}
