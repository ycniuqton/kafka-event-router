import { FilterMode, LookupMode, LookupStepDefaults, LookupStepIssue, LookupStepRules, StepFieldList, TransformStepKind, ValueSourceKind, type TransformStep } from '../../../../domain';

/** Why a step cannot run yet; such steps are skipped by the preview and flagged on their card. */
export const IncompleteReason = {
  Field: 'Choose a field.',
  Target: 'Enter the new field name.',
  Fields: 'List at least one field.',
  Source: 'Enter where the value comes from.',
  Condition: 'Add a condition.',
  Code: 'Write the transform function.',
  Mapping: 'Map at least one output field.',
  Rows: 'Add at least one row or pick a library table.',
  Library: 'Pick a library transform.',
  Header: 'Enter the header name.',
  NewHeader: 'Enter the new header name.',
  LookupSource: LookupStepIssue.Source,
  LookupRequest: LookupStepIssue.Request,
  LookupMapping: LookupStepIssue.Mapping,
  LookupPick: LookupStepIssue.Pick,
  LookupMock: LookupStepIssue.Mock,
} as const;
export type IncompleteReason = (typeof IncompleteReason)[keyof typeof IncompleteReason];

type Check = (step: TransformStep) => IncompleteReason | null;

/** Required options of each step kind, checked before a run so one half-edited step never blocks the others. */
export class StepCompleteness {
  private static readonly field: Check = (s) => (s.field.trim() ? null : IncompleteReason.Field);
  private static readonly fromTo: Check = (s) => StepCompleteness.field(s) ?? (s.argument.trim() ? null : IncompleteReason.Target);
  private static readonly list: Check = (s) => (StepFieldList.of(s).length ? null : IncompleteReason.Fields);
  private static readonly header: Check = (s) => (s.header?.trim() ? null : IncompleteReason.Header);
  private static readonly headerAndField: Check = (s) => StepCompleteness.header(s) ?? StepCompleteness.field(s);
  private static readonly valueSource: Check = (s) => StepCompleteness.field(s) ?? StepCompleteness.sourceText(s);

  private static readonly checks: Record<TransformStepKind, Check> = {
    [TransformStepKind.Rename]: StepCompleteness.fromTo,
    [TransformStepKind.Copy]: StepCompleteness.fromTo,
    [TransformStepKind.Move]: StepCompleteness.fromTo,
    [TransformStepKind.Remove]: StepCompleteness.list,
    [TransformStepKind.KeepOnly]: StepCompleteness.list,
    [TransformStepKind.Mask]: StepCompleteness.field,
    [TransformStepKind.Hash]: StepCompleteness.field,
    [TransformStepKind.ConvertType]: StepCompleteness.field,
    [TransformStepKind.Set]: StepCompleteness.valueSource,
    [TransformStepKind.DefaultIfMissing]: StepCompleteness.field,
    [TransformStepKind.DropIf]: (s) => (StepCompleteness.hasCondition(s) ? null : IncompleteReason.Condition),
    [TransformStepKind.SetKey]: (s) => StepCompleteness.keySource(s),
    [TransformStepKind.Map]: (s) => StepCompleteness.mapping(s),
    [TransformStepKind.Lookup]: (s) => StepCompleteness.lookup(s),
    [TransformStepKind.SplitArray]: StepCompleteness.field,
    [TransformStepKind.Wrap]: StepCompleteness.field,
    [TransformStepKind.Unwrap]: StepCompleteness.field,
    [TransformStepKind.HeaderAdd]: (s) => StepCompleteness.header(s) ?? StepCompleteness.sourceText(s),
    [TransformStepKind.HeaderRemove]: StepCompleteness.header,
    [TransformStepKind.HeaderRename]: (s) => StepCompleteness.header(s) ?? (s.argument.trim() ? null : IncompleteReason.NewHeader),
    [TransformStepKind.HeaderToPayload]: StepCompleteness.headerAndField,
    [TransformStepKind.HeaderFromPayload]: StepCompleteness.headerAndField,
    [TransformStepKind.Library]: (s) => (s.libraryRef && s.libraryVersion ? null : IncompleteReason.Library),
    [TransformStepKind.Js]: (s) => (s.code.trim() ? null : IncompleteReason.Code),
  };

  static reason(step: TransformStep): IncompleteReason | null {
    return StepCompleteness.checks[step.kind](step);
  }

  private static lookup(step: TransformStep): IncompleteReason | null {
    if (step.lookupMode === LookupMode.Source) return LookupStepRules.issue(step.lookupSource ?? LookupStepDefaults.step());
    return StepCompleteness.field(step) ?? StepCompleteness.lookupTable(step);
  }

  private static lookupTable(step: TransformStep): IncompleteReason | null {
    if (step.lookupMode === LookupMode.Table) return step.lookupRef && step.lookupVersion ? null : IncompleteReason.Rows;
    return (step.lookupRows ?? []).some((r) => r.key.trim()) ? null : IncompleteReason.Rows;
  }

  private static sourceText(step: TransformStep): IncompleteReason | null {
    const needsText = (step.sourceKind ?? ValueSourceKind.Constant) !== ValueSourceKind.Constant;
    return needsText && !step.argument.trim() ? IncompleteReason.Source : null;
  }

  private static keySource(step: TransformStep): IncompleteReason | null {
    if ((step.sourceKind ?? ValueSourceKind.Field) === ValueSourceKind.Field) return StepCompleteness.field(step);
    return step.argument.trim() ? null : IncompleteReason.Source;
  }

  private static mapping(step: TransformStep): IncompleteReason | null {
    const entries = step.mapEntries ?? [];
    if (!entries.length) return IncompleteReason.Mapping;
    if (entries.some((e) => !e.path.trim())) return IncompleteReason.Target;
    const open = entries.some((e) => !e.remove && e.sourceKind !== ValueSourceKind.Constant && !e.argument.trim());
    return open ? IncompleteReason.Source : null;
  }

  private static hasCondition(step: TransformStep): boolean {
    const c = step.condition;
    if (!c) return false;
    return c.mode === FilterMode.Expression ? !!c.expression.trim() : c.rows.length > 0;
  }
}
