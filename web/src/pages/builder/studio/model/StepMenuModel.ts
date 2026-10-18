import { JsonFlattener, StepFactory, StepMenu, TransformStepKind, type JsonLine, type StepMenuGroup, type TransformStep } from '../../../../domain';

/** Grouped, searchable "+ Add step" menu and the defaults of a step added from it. */
export class StepMenuModel {
  static readonly kindsWithoutField: readonly TransformStepKind[] = [
    TransformStepKind.Set,
    TransformStepKind.DropIf,
    TransformStepKind.Js,
    TransformStepKind.Wrap,
    TransformStepKind.HeaderAdd,
    TransformStepKind.HeaderRemove,
    TransformStepKind.HeaderRename,
    TransformStepKind.HeaderToPayload,
  ];
  /** Kinds that act on an array: pre-filled with the sample's first array field. */
  static readonly arrayKinds: readonly TransformStepKind[] = [TransformStepKind.SplitArray];
  static readonly listKinds: readonly TransformStepKind[] = [TransformStepKind.Remove, TransformStepKind.KeepOnly];

  static search(query: string): StepMenuGroup[] {
    const needle = query.trim().toLowerCase();
    if (!needle) return StepMenu;
    return StepMenu.map((g) => ({
      group: g.group,
      items: g.items.filter((i) => i.label.toLowerCase().includes(needle) || g.group.toLowerCase().includes(needle)),
    })).filter((g) => g.items.length > 0);
  }

  /** New step pre-filled with the first leaf field of the current sample (design `MENU` onClick). */
  static create(kind: TransformStepKind, inputLines: JsonLine[]): TransformStep {
    const field = StepMenuModel.kindsWithoutField.includes(kind) ? '' : StepMenuModel.firstField(kind, inputLines);
    const fields = StepMenuModel.listKinds.includes(kind) && field ? [field] : undefined;
    return StepFactory.create(kind, { field, fields });
  }

  private static firstField(kind: TransformStepKind, inputLines: JsonLine[]): string {
    const wanted = StepMenuModel.arrayKinds.includes(kind) ? (l: JsonLine) => JsonFlattener.isArray(l) : (l: JsonLine) => !l.isBranch;
    return inputLines.find(wanted)?.path ?? '';
  }
}
