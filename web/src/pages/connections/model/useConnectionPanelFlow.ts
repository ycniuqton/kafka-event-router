import { useEffect, useState } from 'react';
import { ConnectionDisplay, type Connection } from '../../../domain';
import { actions, useCurrentUser } from '../../../store';
import { ConnectionFieldErrors, type ConnectionFieldErrorView } from './ConnectionFieldErrors';
import { ConnectionFormModel, ConnectionFormStep, ConnectionFormStepLabels, type ConnectionFormState } from './ConnectionForm';
import type { ConnectionFormSetter } from './ConnectionFormStepProps';

export type ConnectionPanelFlow = {
  form: ConnectionFormState;
  set: ConnectionFormSetter;
  step: ConnectionFormStep;
  lastStep: number;
  goTo: (step: ConnectionFormStep) => void;
  next: () => void;
  back: () => void;
  saving: boolean;
  testing: boolean;
  errors: ConnectionFieldErrorView;
  testKey: string;
  save: () => Promise<void>;
  testDraft: () => Promise<void>;
};

/** Server round-trips of the panel: Save (create / update) and Test of the unsaved draft, with problem → field mapping. */
function usePanelSubmit(form: ConnectionFormState, editing: Connection | null, onSaved: (id: string) => void, setStep: (s: ConnectionFormStep) => void) {
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [errors, setErrors] = useState<ConnectionFieldErrorView>(ConnectionFieldErrors.none);
  const run = async (setBusy: (busy: boolean) => void, work: () => Promise<void>) => {
    setBusy(true);
    setErrors(ConnectionFieldErrors.none);
    try {
      await work();
    } catch (error) {
      const view = ConnectionFieldErrors.from(error);
      setErrors(view);
      if (view.step !== null) setStep(view.step);
      if (view.stale) void actions.connections.refresh();
      actions.toast.show(view.message);
    } finally {
      setBusy(false);
    }
  };
  const save = () =>
    run(setSaving, async () => {
      const draft = ConnectionFormModel.toDraft(form);
      const saved = editing ? await actions.connections.update(editing.id, draft) : await actions.connections.create(draft);
      onSaved(saved.id);
    });
  const testDraft = () =>
    run(setTesting, async () => {
      await actions.connections.testDraft(ConnectionFormModel.toDraft(form), editing?.id ?? null);
    });
  return { saving, testing, errors, save, testDraft };
}

/**
 * Step machine of the create / edit panel (CN-44): client-side blockers on Next, then server
 * validation on Save or Test, whose problem `path` moves the panel to the failing step (CN-48).
 */
export function useConnectionPanelFlow(editing: Connection | null, takenNames: string[], onSaved: (id: string) => void): ConnectionPanelFlow {
  const user = useCurrentUser();
  const [form, setForm] = useState<ConnectionFormState>(() => (editing ? ConnectionFormModel.fromConnection(editing) : ConnectionFormModel.empty(user?.team)));
  const [step, setStep] = useState<ConnectionFormStep>(ConnectionFormStep.Basics);
  const submit = usePanelSubmit(form, editing, onSaved, setStep);
  const otherNames = takenNames.filter((n) => !editing || n !== ConnectionDisplay.name(editing));
  const set: ConnectionFormSetter = (key, value) => setForm((f) => ({ ...f, [key]: value }));
  const testKey = ConnectionDisplay.draftTestKey(editing?.id ?? null);
  useEffect(() => actions.connections.clearTest(testKey), [testKey]);
  const next = () => {
    const blocker = ConnectionFormModel.blocker(form, step, otherNames);
    if (blocker) actions.toast.show(blocker);
    else setStep((step + 1) as ConnectionFormStep);
  };
  return {
    ...submit,
    form,
    set,
    step,
    lastStep: ConnectionFormStepLabels.length - 1,
    goTo: setStep,
    next,
    back: () => setStep((step - 1) as ConnectionFormStep),
    testKey,
  };
}
