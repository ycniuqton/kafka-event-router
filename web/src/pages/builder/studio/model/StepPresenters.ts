import {
  ConvertTargetLabel,
  FieldPath,
  FilterMode,
  LookupMode,
  MapStart,
  MapStartLabel,
  MaskMode,
  StepFieldList,
  TransformStepKind,
  ValueSourceKind,
  type KafkaEvent,
  type TransformStep,
} from '../../../../domain';
import { StudioText } from './StudioText';

/** Tiny before → after example shown on a recipe card, from the current sample's preview trace. */
export type StepExample = { before: string; after: string };

/** The event entering a step and the event it produced (null when dropped, failed or not run). */
export type StepSnapshots = { before: KafkaEvent | null; after: KafkaEvent | null };

/** Card summary and example of one step kind (design `sum` and `exB/exA`). */
export abstract class StepPresenter {
  abstract readonly kind: TransformStepKind;
  abstract summary(step: TransformStep): string;

  /** Before → after of the step's field on the current sample, read from the preview snapshots. */
  example(step: TransformStep, snapshots: StepSnapshots): StepExample | null {
    if (!snapshots.before || !snapshots.after || !step.field) return null;
    const before = FieldPath.get(snapshots.before.value, step.field);
    const after = FieldPath.get(snapshots.after.value, this.targetPath(step));
    if (after === undefined) return null;
    return { before: this.label(step.field, before), after: this.label(this.targetPath(step), after) };
  }

  protected targetPath(step: TransformStep): string {
    return step.field;
  }

  protected label(_path: string, value: unknown): string {
    return StudioText.exampleValue(value);
  }

  protected field(step: TransformStep): string {
    return step.field || StudioText.missing;
  }
}

/** Steps writing the field's value to another path: example shows `path: value`. */
abstract class FromToPresenter extends StepPresenter {
  summary(step: TransformStep): string {
    return `${this.field(step)} → ${step.argument || StudioText.missing}`;
  }
  protected targetPath(step: TransformStep): string {
    return step.argument;
  }
  protected label(path: string, value: unknown): string {
    return `${path}: ${StudioText.exampleValue(value)}`;
  }
}

class RenamePresenter extends FromToPresenter {
  readonly kind = TransformStepKind.Rename;
}

class CopyPresenter extends FromToPresenter {
  readonly kind = TransformStepKind.Copy;
}

class MovePresenter extends FromToPresenter {
  readonly kind = TransformStepKind.Move;
}

/** Steps on a list of fields; they have no single-value example. */
abstract class FieldListPresenter extends StepPresenter {
  summary(step: TransformStep): string {
    return StepFieldList.format(StepFieldList.of(step)) || StudioText.missing;
  }
  example(): StepExample | null {
    return null;
  }
}

class RemovePresenter extends FieldListPresenter {
  readonly kind = TransformStepKind.Remove;
}

class KeepOnlyPresenter extends FieldListPresenter {
  readonly kind = TransformStepKind.KeepOnly;
}

class MaskPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Mask;
  static readonly modeSummary: Record<MaskMode, string> = {
    [MaskMode.Email]: 'email style',
    [MaskMode.Last4]: 'keep last 4',
    [MaskMode.Full]: 'full',
  };
  summary(step: TransformStep): string {
    return `${this.field(step)} · ${MaskPresenter.modeSummary[step.maskMode]}`;
  }
}

class HashPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Hash;
  summary(step: TransformStep): string {
    return `${this.field(step)} · salted`;
  }
}

class ConvertTypePresenter extends StepPresenter {
  readonly kind = TransformStepKind.ConvertType;
  summary(step: TransformStep): string {
    return step.targetType ? `${this.field(step)} → ${ConvertTargetLabel[step.targetType]}` : this.field(step);
  }
}

/** Steps whose value comes from a constant, another field or record metadata. */
abstract class ValueSourcePresenter extends StepPresenter {
  protected source(step: TransformStep): string {
    const kind = step.sourceKind ?? ValueSourceKind.Constant;
    if (kind === ValueSourceKind.Constant) return step.argument || StudioText.emptyText;
    return `${StudioText.sourcePrefix[kind]}${step.argument || StudioText.missing}`;
  }
}

class SetValuePresenter extends ValueSourcePresenter {
  readonly kind = TransformStepKind.Set;
  summary(step: TransformStep): string {
    return `${this.field(step)} = ${this.source(step)}`;
  }
}

class DefaultIfMissingPresenter extends ValueSourcePresenter {
  readonly kind = TransformStepKind.DefaultIfMissing;
  summary(step: TransformStep): string {
    return `${this.field(step)} ?? ${this.source(step)}`;
  }
}

class SetKeyPresenter extends ValueSourcePresenter {
  readonly kind = TransformStepKind.SetKey;
  summary(step: TransformStep): string {
    const kind = step.sourceKind ?? ValueSourceKind.Field;
    return `key ← ${kind === ValueSourceKind.Field ? this.field(step) : this.source(step)}`;
  }
  example(_step: TransformStep, snapshots: StepSnapshots): StepExample | null {
    if (!snapshots.before || !snapshots.after) return null;
    return { before: StudioText.keyExample(snapshots.before.key), after: StudioText.keyExample(snapshots.after.key) };
  }
}

class DropIfPresenter extends StepPresenter {
  readonly kind = TransformStepKind.DropIf;
  summary(step: TransformStep): string {
    const c = step.condition;
    if (!c) return StudioText.missing;
    if (c.mode === FilterMode.Expression) return c.expression || StudioText.missing;
    return c.rows.map((r) => `${FieldPath.short(r.field)} ${r.op} ${r.value}`).join(` ${c.combinator} `) || StudioText.missing;
  }
  example(): StepExample | null {
    return null;
  }
}

class MapPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Map;
  summary(step: TransformStep): string {
    return StudioText.mapSummary(MapStartLabel[step.mapStart ?? MapStart.CopyInput], step.mapEntries?.length ?? 0);
  }
  example(): StepExample | null {
    return null;
  }
}

class LookupPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Lookup;
  summary(step: TransformStep): string {
    const table = step.lookupMode === LookupMode.Table ? StudioText.lookupTable(step.lookupRef, step.lookupVersion) : StudioText.lookupRows(step.lookupRows?.length ?? 0);
    return `${this.field(step)} → ${step.argument || this.field(step)} · ${table}`;
  }
  protected targetPath(step: TransformStep): string {
    return step.argument || step.field;
  }
}

class SplitArrayPresenter extends StepPresenter {
  readonly kind = TransformStepKind.SplitArray;
  summary(step: TransformStep): string {
    return `${this.field(step)}[] → ${step.argument || this.field(step)} · ${StudioText.eachItem}`;
  }
  example(step: TransformStep, snapshots: StepSnapshots): StepExample | null {
    if (!snapshots.before || !snapshots.after || !step.field) return null;
    const items = FieldPath.get(snapshots.before.value, step.field);
    const target = step.argument || step.field;
    if (!Array.isArray(items)) return null;
    return { before: StudioText.itemCount(step.field, items.length), after: `${target}: ${StudioText.exampleValue(FieldPath.get(snapshots.after.value, target))}` };
  }
}

/** Wrap / unwrap move the whole value under a member path or back; examples show the value on both sides. */
abstract class MemberPresenter extends StepPresenter {
  protected abstract readonly inward: boolean;
  summary(step: TransformStep): string {
    const member = this.field(step);
    return this.inward ? `${StudioText.wholeValue} → ${member}` : `${member} → ${StudioText.wholeValue}`;
  }
  example(step: TransformStep, snapshots: StepSnapshots): StepExample | null {
    if (!snapshots.before || !snapshots.after || !step.field) return null;
    const inner = this.inward ? snapshots.after : snapshots.before;
    const whole = this.inward ? snapshots.before : snapshots.after;
    const member = `${step.field}: ${StudioText.exampleValue(FieldPath.get(inner.value, step.field))}`;
    const value = `${StudioText.wholeValue}: ${StudioText.exampleValue(whole.value)}`;
    return this.inward ? { before: value, after: member } : { before: member, after: value };
  }
}

class WrapPresenter extends MemberPresenter {
  readonly kind = TransformStepKind.Wrap;
  protected readonly inward = true;
}

class UnwrapPresenter extends MemberPresenter {
  readonly kind = TransformStepKind.Unwrap;
  protected readonly inward = false;
}

/** One side of a header step example: a header by name or a payload field. */
type HeaderSide = { header?: string; field?: string };

/** Header steps: summary and before → after of the header (or payload field) on each side. */
abstract class HeaderPresenter extends StepPresenter {
  protected abstract from(step: TransformStep): HeaderSide;
  protected abstract to(step: TransformStep): HeaderSide;

  example(step: TransformStep, snapshots: StepSnapshots): StepExample | null {
    if (!snapshots.before || !snapshots.after || !step.header) return null;
    return { before: this.side(snapshots.before, this.from(step)), after: this.side(snapshots.after, this.to(step)) };
  }

  protected header(step: TransformStep): string {
    return step.header || StudioText.missing;
  }

  private side(event: KafkaEvent, side: HeaderSide): string {
    if (side.field !== undefined) return `${side.field}: ${StudioText.exampleValue(FieldPath.get(event.value, side.field))}`;
    return StudioText.headerExample(side.header ?? '', event.headers[side.header ?? '']);
  }
}

/** Header steps changing one header in place (add / remove). */
abstract class SameHeaderPresenter extends HeaderPresenter {
  protected from(step: TransformStep): HeaderSide {
    return { header: step.header };
  }
  protected to(step: TransformStep): HeaderSide {
    return { header: step.header };
  }
}

class HeaderAddPresenter extends SameHeaderPresenter {
  readonly kind = TransformStepKind.HeaderAdd;
  summary(step: TransformStep): string {
    const kind = step.sourceKind ?? ValueSourceKind.Constant;
    const source = kind === ValueSourceKind.Constant ? step.argument || StudioText.emptyText : `${StudioText.sourcePrefix[kind]}${step.argument || StudioText.missing}`;
    return `${this.header(step)} ← ${source}`;
  }
}

class HeaderRemovePresenter extends SameHeaderPresenter {
  readonly kind = TransformStepKind.HeaderRemove;
  summary(step: TransformStep): string {
    return `− ${this.header(step)}`;
  }
}

class HeaderRenamePresenter extends HeaderPresenter {
  readonly kind = TransformStepKind.HeaderRename;
  summary(step: TransformStep): string {
    return `${this.header(step)} → ${step.argument || StudioText.missing}`;
  }
  protected from(step: TransformStep): HeaderSide {
    return { header: step.header };
  }
  protected to(step: TransformStep): HeaderSide {
    return { header: step.argument };
  }
}

class HeaderToPayloadPresenter extends HeaderPresenter {
  readonly kind = TransformStepKind.HeaderToPayload;
  summary(step: TransformStep): string {
    return `${this.header(step)} → ${this.field(step)}`;
  }
  protected from(step: TransformStep): HeaderSide {
    return { header: step.header };
  }
  protected to(step: TransformStep): HeaderSide {
    return { field: step.field };
  }
}

class HeaderFromPayloadPresenter extends HeaderPresenter {
  readonly kind = TransformStepKind.HeaderFromPayload;
  summary(step: TransformStep): string {
    return `${this.field(step)} → ${this.header(step)}`;
  }
  protected from(step: TransformStep): HeaderSide {
    return { field: step.field };
  }
  protected to(step: TransformStep): HeaderSide {
    return { header: step.header };
  }
}

class LibraryPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Library;
  summary(step: TransformStep): string {
    return StudioText.lookupTable(step.libraryRef, step.libraryVersion);
  }
  example(): StepExample | null {
    return null;
  }
}

class JsPresenter extends StepPresenter {
  readonly kind = TransformStepKind.Js;
  summary(step: TransformStep): string {
    return `${step.code.split(StudioText.lineBreak).length} lines`;
  }
  example(): StepExample | null {
    return null;
  }
}

/** Manager of step presenters keyed by kind; register a presenter to support a new step kind. */
export class StepPresenters {
  private static readonly byKind = new Map<TransformStepKind, StepPresenter>(
    [
      new RenamePresenter(),
      new CopyPresenter(),
      new MovePresenter(),
      new RemovePresenter(),
      new KeepOnlyPresenter(),
      new MaskPresenter(),
      new HashPresenter(),
      new ConvertTypePresenter(),
      new SetValuePresenter(),
      new DefaultIfMissingPresenter(),
      new SetKeyPresenter(),
      new DropIfPresenter(),
      new MapPresenter(),
      new LookupPresenter(),
      new SplitArrayPresenter(),
      new WrapPresenter(),
      new UnwrapPresenter(),
      new HeaderAddPresenter(),
      new HeaderRemovePresenter(),
      new HeaderRenamePresenter(),
      new HeaderToPayloadPresenter(),
      new HeaderFromPayloadPresenter(),
      new LibraryPresenter(),
      new JsPresenter(),
    ].map((p) => [p.kind, p]),
  );

  static of(kind: TransformStepKind): StepPresenter {
    return StepPresenters.byKind.get(kind) as StepPresenter;
  }
}
