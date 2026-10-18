import type { ComponentType } from 'react';
import { Drawer } from '../../../components/ui';
import { ConnectionDisplay, type Connection } from '../../../domain';
import { useConnectionTest } from '../../../store';
import { ConnectionDrawerTitle } from '../model/ConnectionDrawer';
import { ConnectionFormStep } from '../model/ConnectionForm';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { useConnectionPanelFlow } from '../model/useConnectionPanelFlow';
import { TestChecklist } from '../test/TestChecklist';
import { BasicsStep } from './BasicsStep';
import { BrokersStep } from './BrokersStep';
import { FormFooter } from './FormFooter';
import { FormStepper } from './FormStepper';
import { SchemaRegistryStep } from './SchemaRegistryStep';
import { SecurityStep } from './SecurityStep';

/** Which view renders each step of the panel. */
const StepViews: Record<ConnectionFormStep, ComponentType<ConnectionFormStepProps>> = {
  [ConnectionFormStep.Basics]: BasicsStep,
  [ConnectionFormStep.Brokers]: BrokersStep,
  [ConnectionFormStep.Security]: SecurityStep,
  [ConnectionFormStep.SchemaRegistry]: SchemaRegistryStep,
};

export type ConnectionFormDrawerProps = {
  editing: Connection | null;
  takenNames: string[];
  onClose: () => void;
  onSaved: (connectionId: string) => void;
};

/** The 4-step create / edit connection side panel (spec §5.2); Test checks the unsaved draft, saving runs the test. */
export function ConnectionFormDrawer({ editing, takenNames, onClose, onSaved }: ConnectionFormDrawerProps) {
  const flow = useConnectionPanelFlow(editing, takenNames, onSaved);
  const draftRun = useConnectionTest(flow.testKey);
  const StepView = StepViews[flow.step];
  const footer = (
    <FormFooter
      step={flow.step}
      lastStep={flow.lastStep}
      saving={flow.saving}
      testing={flow.testing}
      onBack={flow.back}
      onNext={flow.next}
      onTest={() => void flow.testDraft()}
      onSave={() => void flow.save()}
    />
  );
  return (
    <Drawer open onClose={onClose} title={editing ? ConnectionDrawerTitle.editPrefix + ConnectionDisplay.name(editing) : ConnectionDrawerTitle.create} footer={footer}>
      <FormStepper step={flow.step} onSelect={(i) => flow.goTo(i as ConnectionFormStep)} />
      <StepView form={flow.form} set={flow.set} editing={!!editing} connectionId={editing?.id} errors={flow.errors.fields} />
      <TestChecklist run={draftRun} />
    </Drawer>
  );
}
