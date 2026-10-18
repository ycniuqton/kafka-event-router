import { describe, expect, it } from 'vitest';
import { MockJsExporter } from '../../../../api/mock/MockJsExporter';
import { MaskMode, StepFactory, TransformRunner, TransformStepKind, ValueSourceKind } from '../../../../domain';
import { StepListEditor } from '../model/StepListEditor';
import { ExportSelection } from './ExportSelection';

const steps = [
  StepFactory.create(TransformStepKind.Rename, { field: 'a', argument: 'x.b' }),
  StepFactory.create(TransformStepKind.Js, { code: 'function transform(e){return e}' }),
  StepFactory.create(TransformStepKind.Set, { field: 'c', sourceKind: ValueSourceKind.Constant, argument: '42' }),
  StepFactory.create(TransformStepKind.Mask, { field: 'email', maskMode: MaskMode.Full }),
];

describe('Export to JS', () => {
  it('selects recipe steps only, in pipeline order', () => {
    const sel = new ExportSelection([steps[3].id, steps[1].id, steps[0].id]);
    expect(sel.steps(steps).map((s) => s.kind)).toEqual([TransformStepKind.Rename, TransformStepKind.Mask]);
    expect(new ExportSelection(sel.toggle(steps[0].id)).has(steps[0].id)).toBe(false);
  });

  it('replaces the selection by one js step where the first selected step was', () => {
    const js = StepFactory.create(TransformStepKind.Js, { code: 'x' });
    const edit = new StepListEditor(steps).replace(new Set([steps[2].id, steps[0].id]), js, 'Exported');
    expect(edit.steps.map((s) => s.id)).toEqual([js.id, steps[1].id, steps[3].id]);
  });

  it('mock export produces JS with the same result as the recipes', () => {
    const recipes = [steps[0], steps[2], steps[3]];
    const event = { key: '', headers: {}, value: { a: 1, email: 'a@b.c' } };
    const viaRecipes = TransformRunner.run(event, recipes);
    const viaJs = TransformRunner.run(event, [MockJsExporter.export(recipes)]);
    expect(viaJs.error).toBeUndefined();
    expect(viaJs.event?.value).toEqual(viaRecipes.event?.value);
  });
});
