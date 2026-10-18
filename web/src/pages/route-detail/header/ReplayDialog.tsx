import { useState } from 'react';
import { Button, Field, Modal, TextInput } from '../../../components/ui';
import { RouteEndpointText, Tone, type Replay, type Route } from '../../../domain';
import { useNow } from '../../../store';
import { ReplayForm, type ReplayErrors, type ReplayInput, type ReplayStarter } from './ReplayForm';
import styles from './ReplayDialog.module.css';

type ReplayDialogProps = { route: Route; onClose: () => void; onStarted: (replay: Replay) => void; start: ReplayStarter };

/** Replay confirm: a start time in the past, the duplicate-delivery warning, and the route name typed to confirm. */
export function ReplayDialog({ route, onClose, onStarted, start }: ReplayDialogProps) {
  const now = useNow();
  const [input, setInput] = useState<ReplayInput>(() => ({ start: ReplayForm.defaultStart(now), acknowledged: false, typedName: '' }));
  const [serverErrors, setServerErrors] = useState<ReplayErrors>({});
  const [busy, setBusy] = useState(false);
  const destination = RouteEndpointText.destination(route);
  const errors = { ...ReplayForm.validate(input, now, route.name), ...serverErrors };
  const update = (patch: Partial<ReplayInput>) => {
    setServerErrors({});
    setInput((prev) => ({ ...prev, ...patch }));
  };
  const confirm = () => {
    setBusy(true);
    void ReplayForm.submit(input, now, route.name, start)
      .then((outcome) => (outcome.replay ? onStarted(outcome.replay) : setServerErrors(outcome.errors)))
      .finally(() => setBusy(false));
  };
  const footer = (
    <>
      <Button size="lg" onClick={onClose}>Cancel</Button>
      <Button size="lg" variant="danger" onClick={confirm} disabled={busy || !ReplayForm.canSubmit(input, now, route.name)}>
        Replay
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title="Replay events" subtitle={`Re-read ${route.source.topics.join(' + ')} from a point in time`} footer={footer}>
      <div className={styles.body}>
        <Field label="Start from (UTC)" error={errors.from}>
          <TextInput type="datetime-local" mono value={input.start} onChange={(e) => update({ start: e.target.value })} aria-label="Start from (UTC)" />
        </Field>
        <div className={styles.warning} data-tone={Tone.Warn}>
          <strong>The destination will receive duplicates.</strong> Events already delivered to <span className={styles.mono}>{destination}</span> since
          this time are sent again. Make sure consumers can handle repeats before you continue.
        </div>
        <label className={styles.ack}>
          <input type="checkbox" checked={input.acknowledged} onChange={(e) => update({ acknowledged: e.target.checked })} />
          <span>I understand that consumers of {destination} will see these events twice.</span>
        </label>
        <Field label={`Type ${route.name} to confirm`} error={input.typedName ? errors.name : undefined}>
          <TextInput mono value={input.typedName} onChange={(e) => update({ typedName: e.target.value })} aria-label="Route name" />
        </Field>
        {errors.form && (
          <span className={styles.formError} data-tone={Tone.Err}>
            {errors.form}
          </span>
        )}
      </div>
    </Modal>
  );
}
