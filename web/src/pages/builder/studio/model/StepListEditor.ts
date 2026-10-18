import { StepFactory, type TransformStep } from '../../../../domain';
import { StepChangeLabel } from './StudioText';

/** A new step list plus the change label reported to the builder. */
export type StepEdit = { steps: TransformStep[]; label?: string };

/** Immutable edits of the step list (add, patch, toggle, move, duplicate, delete). */
export class StepListEditor {
  private readonly steps: TransformStep[];

  constructor(steps: TransformStep[]) {
    this.steps = steps;
  }

  append(added: TransformStep[], label: string): StepEdit {
    return { steps: [...this.steps, ...added], label };
  }

  /** Inserts after the step at `index` (appends when no step is selected). */
  insertAfter(index: number, added: TransformStep[], label: string): StepEdit {
    if (index < 0) return this.append(added, label);
    return { steps: [...this.steps.slice(0, index + 1), ...added, ...this.steps.slice(index + 1)], label };
  }

  patch(id: number, patch: Partial<TransformStep>, label?: string): StepEdit {
    return { steps: this.steps.map((s) => (s.id === id ? { ...s, ...patch } : s)), label };
  }

  toggle(step: TransformStep): StepEdit {
    return this.patch(step.id, { enabled: !step.enabled }, StepChangeLabel.toggled(step.kind, step.enabled));
  }

  remove(step: TransformStep): StepEdit {
    return { steps: this.steps.filter((s) => s.id !== step.id), label: StepChangeLabel.removed(step.kind) };
  }

  duplicate(step: TransformStep): StepEdit {
    const { id: _id, ...rest } = step;
    const copy = StepFactory.create(step.kind, rest);
    const at = this.steps.findIndex((s) => s.id === step.id);
    const steps = [...this.steps.slice(0, at + 1), copy, ...this.steps.slice(at + 1)];
    return { steps, label: StepChangeLabel.duplicated(step.kind) };
  }

  /** Replaces the steps `ids` by `step`, placed where the first of them was (export to JS). */
  replace(ids: ReadonlySet<number>, step: TransformStep, label: string): StepEdit {
    const at = this.steps.findIndex((s) => ids.has(s.id));
    const kept = this.steps.filter((s) => !ids.has(s.id));
    const before = this.steps.slice(0, Math.max(at, 0)).filter((s) => !ids.has(s.id)).length;
    return { steps: [...kept.slice(0, before), step, ...kept.slice(before)], label };
  }

  /** Moves the step at `from` to position `to`; null when nothing changes. */
  move(from: number, to: number): StepEdit | null {
    if (from === to || to < 0 || to >= this.steps.length) return null;
    const steps = this.steps.slice();
    const [moved] = steps.splice(from, 1);
    steps.splice(to, 0, moved);
    return { steps, label: StepChangeLabel.reordered };
  }
}
