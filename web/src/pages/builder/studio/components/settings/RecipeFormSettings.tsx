import { Fragment, type ComponentType } from 'react';
import { ControlSize, TextInput } from '../../../../../components/ui';
import { ValueSourceKind, type TransformStep } from '../../../../../domain';
import { useStudio } from '../../hooks/StudioContext';
import { RecipeFormSpec, RecipeInput, type RecipeFormInput } from '../../model/RecipeFormSpec';
import { StepChangeLabel } from '../../model/StudioText';
import { FieldSelect } from './FieldSelect';
import type { StepSettingsProps } from './StepSettingsProps';
import { ValueSourceInput } from './ValueSourceInput';
import styles from './RecipeSettings.module.css';

type TextInputKind = Exclude<RecipeInput, typeof RecipeInput.Source>;
type InputProps = { step: TransformStep; spec: RecipeFormInput & { input: TextInputKind }; onChange(patch: Partial<TransformStep>): void };

/** Header name with suggestions from the current sample's headers. */
function HeaderNameInput({ step, spec, onChange }: InputProps) {
  const { model } = useStudio();
  const listId = `step-${step.id}-${spec.input}-headers`;
  return (
    <>
      <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} list={listId} placeholder={spec.placeholder} aria-label={spec.label}
        value={RecipeFormSpec.read(step, spec.input)} onChange={(e) => onChange(RecipeFormSpec.patch(spec.input, e.target.value))} />
      <datalist id={listId}>
        {Object.keys(model.current?.headers ?? {}).map((h) => (
          <option key={h} value={h} />
        ))}
      </datalist>
    </>
  );
}

function PathInput({ step, spec, onChange }: InputProps) {
  return (
    <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={spec.placeholder} aria-label={spec.label}
      value={RecipeFormSpec.read(step, spec.input)} onChange={(e) => onChange(RecipeFormSpec.patch(spec.input, e.target.value))} />
  );
}

function FieldPickInput({ step, spec, onChange }: InputProps) {
  return <FieldSelect className={styles.grow} ariaLabel={spec.label} value={RecipeFormSpec.read(step, spec.input)} onChange={(v) => onChange(RecipeFormSpec.patch(spec.input, v))} />;
}

/** Control of each text input kind. */
const RecipeInputControl: Record<TextInputKind, ComponentType<InputProps>> = {
  [RecipeInput.FieldPick]: FieldPickInput,
  [RecipeInput.FieldPath]: PathInput,
  [RecipeInput.Target]: PathInput,
  [RecipeInput.Header]: HeaderNameInput,
  [RecipeInput.NewHeader]: HeaderNameInput,
};

/** Structure (split array, wrap, unwrap) and header recipes: one row of inputs from `RecipeFormSpec`, value source below. */
export function RecipeFormSettings({ step, update }: StepSettingsProps) {
  const label = StepChangeLabel.edited(step.kind);
  const form = RecipeFormSpec.of(step.kind);
  const row = form.inputs.filter((i): i is InputProps['spec'] => i.input !== RecipeInput.Source);
  const hasSource = form.inputs.some((i) => i.input === RecipeInput.Source);
  const onChange = (patch: Partial<TransformStep>) => update(patch, label);
  return (
    <>
      <div className={styles.row}>
        {row.map((spec, i) => {
          const Control = RecipeInputControl[spec.input];
          return (
            <Fragment key={spec.input}>
              {i > 0 && <span className={styles.op}>{form.op}</span>}
              <Control step={step} spec={spec} onChange={onChange} />
            </Fragment>
          );
        })}
        {hasSource && <span className={styles.op}>{form.op}</span>}
      </div>
      {hasSource && <ValueSourceInput step={step} fallback={ValueSourceKind.Constant} onChange={onChange} />}
    </>
  );
}
