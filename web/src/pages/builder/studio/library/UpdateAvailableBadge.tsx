import { Button, ButtonSize, ButtonVariant, ToneBadge } from '../../../../components/ui';
import { LibraryUpdates, Tone, TransformStepKind, type TransformStep } from '../../../../domain';
import { LibraryPin, LibraryPinText } from './LibraryPin';
import { useStudio } from '../hooks/StudioContext';
import { StepListEditor } from '../model/StepListEditor';
import { useLibraryLatest } from './LibraryLatest';

/** "Update available" badge with a manual Update button; steps never move to a new version by themselves. */
export function UpdateAvailableBadge({ step }: { step: TransformStep }) {
  const { model, commands } = useStudio();
  const latest = useLibraryLatest();
  const pin = LibraryPin.of(step);
  if (!pin) return null;
  const versions = step.kind === TransformStepKind.Library ? latest.transforms : latest.tables;
  const newer = LibraryUpdates.available(pin.version, versions[pin.name]);
  if (newer === null) return null;
  const update = () => commands.commit(new StepListEditor(model.steps).patch(step.id, pin.patch(newer), LibraryPinText.updated(pin.name, newer)));
  return (
    <span onClick={(e) => e.stopPropagation()}>
      <ToneBadge tone={Tone.Warn}>{LibraryPinText.available(newer)}</ToneBadge>{' '}
      {!model.readOnly && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={update}>
          {LibraryPinText.update}
        </Button>
      )}
    </span>
  );
}
