import type { TransformStep } from '../../../../../domain';

/** Props of every per-kind settings editor; `update` reports the change label to the builder. */
export type StepSettingsProps = {
  step: TransformStep;
  index: number;
  update(patch: Partial<TransformStep>, label?: string): void;
};
