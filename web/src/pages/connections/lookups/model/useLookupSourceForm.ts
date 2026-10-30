import { useState } from 'react';
import { services } from '../../../../api';
import { LookupSourceDefaults, LookupSourceRules, type LookupFieldErrors, type LookupSource, type LookupSourceDraft, type LookupTestResult } from '../../../../domain';
import { LookupErrorSteps, LookupFormStep } from './LookupSourceForm';

/** Saved source and the result of the test run right after saving (null when the test call failed). */
export type LookupSaved = { source: LookupSource; test: LookupTestResult | null };

/** Outcome of a save attempt: saved, field errors (pointers), or a general failure message. */
type SaveOutcome = { saved?: LookupSaved; errors?: LookupFieldErrors; failure?: string };

/** Save & test and draft tests over `services.lookupSources`; errors become field errors or a message. */
export class LookupSourceSaver {
  static async save(editing: LookupSource | null, draft: LookupSourceDraft): Promise<SaveOutcome> {
    try {
      const source = editing ? await services.lookupSources.update(editing.id, draft) : await services.lookupSources.create(draft);
      const test = await services.lookupSources.test(source.id).catch(() => null);
      return { saved: { source, test } };
    } catch (error) {
      const fields = LookupSourceSaver.fieldErrors(error);
      return fields ? { errors: fields } : { failure: (error as Error).message };
    }
  }

  static async test(draft: LookupSourceDraft): Promise<{ test?: LookupTestResult; failure?: string }> {
    try {
      return { test: await services.lookupSources.testDraft(draft) };
    } catch (error) {
      return { failure: (error as Error).message };
    }
  }

  private static fieldErrors(error: unknown): LookupFieldErrors | null {
    const withFields = error as { fieldErrors?: () => Record<string, string> };
    const fields = typeof withFields.fieldErrors === 'function' ? withFields.fieldErrors() : {};
    return Object.keys(fields).length ? fields : null;
  }
}

export type LookupSourceFormFlow = {
  draft: LookupSourceDraft;
  step: LookupFormStep;
  errors: LookupFieldErrors;
  failure: string | null;
  busy: boolean;
  test: LookupTestResult | null;
  update: (next: LookupSourceDraft) => void;
  goTo: (step: LookupFormStep) => void;
  next: () => void;
  back: () => void;
  testDraft: () => Promise<void>;
  save: () => Promise<void>;
};

/** Panel flow: per-step validation on Next, full validation + save + test on Save & test, draft test any time. */
export function useLookupSourceForm(editing: LookupSource | null, onSaved: (saved: LookupSaved) => void): LookupSourceFormFlow {
  const [draft, setDraft] = useState<LookupSourceDraft>(() => (editing ? LookupSourceDefaults.fromSource(editing) : LookupSourceDefaults.draft()));
  const [step, setStep] = useState<LookupFormStep>(LookupFormStep.Basics);
  const [errors, setErrors] = useState<LookupFieldErrors>({});
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [test, setTest] = useState<LookupTestResult | null>(null);
  const validate = () => LookupSourceRules.validate(draft, editing?.secretFields ?? []);
  const next = () => {
    const stepErrors = LookupErrorSteps.onStep(validate(), step);
    setErrors(stepErrors);
    if (!Object.keys(stepErrors).length) setStep(Math.min(step + 1, LookupFormStep.Limits) as LookupFormStep);
  };
  const run = async <T,>(work: () => Promise<T>): Promise<T> => {
    setBusy(true);
    try {
      return await work();
    } finally {
      setBusy(false);
    }
  };
  const testDraft = async () => {
    const out = await run(() => LookupSourceSaver.test(draft));
    setTest(out.test ?? null);
    setFailure(out.failure ?? null);
  };
  const save = async () => {
    const all = validate();
    setErrors(all);
    const first = LookupErrorSteps.firstStep(all);
    if (first !== null) return setStep(first);
    const out = await run(() => LookupSourceSaver.save(editing, draft));
    if (out.saved) return onSaved(out.saved);
    setErrors(out.errors ?? {});
    setFailure(out.failure ?? null);
  };
  const update = (d: LookupSourceDraft) => {
    setDraft(d);
    setFailure(null);
  };
  const back = () => setStep(Math.max(step - 1, LookupFormStep.Basics) as LookupFormStep);
  return { draft, step, errors, failure, busy, test, update, goTo: setStep, next, back, testDraft, save };
}
