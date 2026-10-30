import type { ComponentType } from 'react';
import { Drawer } from '../../../components/ui';
import type { LookupSource } from '../../../domain';
import { FormFooter } from '../components/FormFooter';
import { FormStepper } from '../components/FormStepper';
import { LookupBasicsStep } from './LookupBasicsStep';
import { LookupKindStep, LookupLimitsStep } from './LookupKindStep';
import { LookupTestChecklist } from './LookupTestChecklist';
import { LookupFormStep, LookupFormStepLabels, type LookupFormStepProps } from './model/LookupSourceForm';
import { useLookupSourceForm, type LookupSaved } from './model/useLookupSourceForm';

/** Which view renders each step of the panel. */
const LookupStepViews: Record<LookupFormStep, ComponentType<LookupFormStepProps>> = {
  [LookupFormStep.Basics]: LookupBasicsStep,
  [LookupFormStep.Kind]: LookupKindStep,
  [LookupFormStep.Limits]: LookupLimitsStep,
};

/** Drawer titles. */
const LookupDrawerTitle = { create: 'New lookup source', editPrefix: 'Edit ' } as const;

export type LookupSourceFormDrawerProps = { editing: LookupSource | null; onClose: () => void; onSaved: (saved: LookupSaved) => void };

/** Create / edit a lookup source: Basics → Kind settings → Limits; Test checks the unsaved draft, saving runs the test. */
export function LookupSourceFormDrawer({ editing, onClose, onSaved }: LookupSourceFormDrawerProps) {
  const flow = useLookupSourceForm(editing, onSaved);
  const StepView = LookupStepViews[flow.step];
  const footer = (
    <FormFooter step={flow.step} lastStep={LookupFormStep.Limits} saving={flow.busy} testing={flow.busy}
      onBack={flow.back} onNext={flow.next} onTest={() => void flow.testDraft()} onSave={() => void flow.save()} />
  );
  return (
    <Drawer open onClose={onClose} title={editing ? LookupDrawerTitle.editPrefix + editing.name : LookupDrawerTitle.create} footer={footer}>
      <FormStepper step={flow.step} labels={LookupFormStepLabels} onSelect={(i) => flow.goTo(i as LookupFormStep)} />
      <StepView draft={flow.draft} stored={editing?.secretFields ?? []} editing={!!editing} teamName={editing?.owner.teamName ?? ''} errors={flow.errors} update={flow.update} />
      <LookupTestChecklist result={flow.test} failure={flow.failure} />
    </Drawer>
  );
}
