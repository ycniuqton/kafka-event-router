import type { KafkaEvent } from '../event';
import { LookupMode, TransformStepKind, type TransformStep } from '../transform';
import { MapStart } from '../mapper';
import { ConvertFailure, ConvertTarget, StepConstant, StepFieldList, ValueSourceKind } from '../transformOptions';
import { FieldPath } from './FieldPath';
import { LookupAnswerApplier } from './LookupAnswerApplier';
import { FilterEvaluator } from './FilterEvaluator';
import { JsSandbox } from './JsSandbox';
import { ValueHasher, ValueMasker } from './Privacy';

type Payload = Record<string, unknown>;

/** Applies one kind of transform step to an event; returns null to drop it. */
export abstract class StepHandler {
  abstract readonly kind: TransformStepKind;
  abstract apply(event: KafkaEvent, step: TransformStep, logs: string[]): KafkaEvent | null;

  protected payload(event: KafkaEvent): Payload {
    return event.value as Payload;
  }
}

class RenameHandler extends StepHandler {
  readonly kind = TransformStepKind.Rename;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const v = this.payload(event);
    const current = FieldPath.get(v, step.field);
    if (current === undefined || !step.argument) return event;
    FieldPath.remove(v, step.field);
    FieldPath.set(v, step.argument, current);
    return event;
  }
}

class MaskHandler extends StepHandler {
  readonly kind = TransformStepKind.Mask;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const v = this.payload(event);
    const current = FieldPath.get(v, step.field);
    if (current !== undefined) FieldPath.set(v, step.field, ValueMasker.mask(current, step.maskMode));
    return event;
  }
}

class HashHandler extends StepHandler {
  readonly kind = TransformStepKind.Hash;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const v = this.payload(event);
    const current = FieldPath.get(v, step.field);
    if (current !== undefined) FieldPath.set(v, step.field, ValueHasher.hash(current));
    return event;
  }
}

class RemoveHandler extends StepHandler {
  readonly kind = TransformStepKind.Remove;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    StepFieldList.of(step).forEach((path) => FieldPath.remove(this.payload(event), path));
    return event;
  }
}

/** Reads a step's value source (constant, field or metadata) in the mock engine. */
class MockValueSource {
  static read(event: KafkaEvent, kind: ValueSourceKind | undefined, text: string): unknown {
    if (kind === ValueSourceKind.Field) return FieldPath.get(event.value, text);
    if (kind === ValueSourceKind.Metadata) return (event as unknown as Record<string, unknown>)[text];
    if (kind === ValueSourceKind.Formula) return undefined;
    return StepConstant.parse(text);
  }
}

class SetValueHandler extends StepHandler {
  readonly kind: TransformStepKind = TransformStepKind.Set;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    if (!step.field || !this.applies(event, step)) return event;
    FieldPath.set(this.payload(event), step.field, MockValueSource.read(event, step.sourceKind, step.argument));
    return event;
  }

  protected applies(_event: KafkaEvent, _step: TransformStep): boolean {
    return true;
  }
}

class DefaultIfMissingHandler extends SetValueHandler {
  readonly kind = TransformStepKind.DefaultIfMissing;
  protected applies(event: KafkaEvent, step: TransformStep): boolean {
    return FieldPath.get(this.payload(event), step.field) == null;
  }
}

class SetKeyHandler extends StepHandler {
  readonly kind = TransformStepKind.SetKey;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const kind = step.sourceKind ?? ValueSourceKind.Field;
    const current = MockValueSource.read(event, kind, kind === ValueSourceKind.Field ? step.field : step.argument);
    if (current !== undefined) event.key = String(current);
    return event;
  }
}

class CopyHandler extends StepHandler {
  readonly kind: TransformStepKind = TransformStepKind.Copy;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const v = this.payload(event);
    const current = FieldPath.get(v, step.field);
    if (current === undefined || !step.argument) return event;
    FieldPath.set(v, step.argument, structuredClone(current));
    if (this.removesSource) FieldPath.remove(v, step.field);
    return event;
  }

  protected get removesSource(): boolean {
    return false;
  }
}

class MoveHandler extends CopyHandler {
  readonly kind = TransformStepKind.Move;
  protected get removesSource(): boolean {
    return true;
  }
}

class KeepOnlyHandler extends StepHandler {
  readonly kind = TransformStepKind.KeepOnly;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const kept: Payload = {};
    for (const path of StepFieldList.of(step)) {
      const current = FieldPath.get(this.payload(event), path);
      if (current !== undefined) FieldPath.set(kept, path, current);
    }
    return { ...event, value: kept as KafkaEvent['value'] };
  }
}

/** Type conversions of the convert-type recipe in the mock engine. */
class MockConverter {
  static readonly convert: Record<ConvertTarget, (v: unknown) => unknown> = {
    [ConvertTarget.String]: (v) => (typeof v === 'object' ? JSON.stringify(v) : String(v)),
    [ConvertTarget.Number]: (v) => MockConverter.number(Number(v)),
    [ConvertTarget.Integer]: (v) => MockConverter.number(Math.trunc(Number(v))),
    [ConvertTarget.Boolean]: (v) => MockConverter.boolean(v),
  };
  static readonly truthy = ['true', '1', 'yes'];
  static readonly falsy = ['false', '0', 'no'];

  private static number(n: number): number {
    if (Number.isNaN(n)) throw new Error('value is not a number');
    return n;
  }

  private static boolean(v: unknown): boolean {
    const text = String(v).toLowerCase();
    if (MockConverter.truthy.includes(text)) return true;
    if (MockConverter.falsy.includes(text)) return false;
    throw new Error('value is not a boolean');
  }
}

class ConvertTypeHandler extends StepHandler {
  readonly kind = TransformStepKind.ConvertType;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const v = this.payload(event);
    const current = FieldPath.get(v, step.field);
    if (current === undefined) return event;
    try {
      FieldPath.set(v, step.field, MockConverter.convert[step.targetType ?? ConvertTarget.Number](current));
    } catch (e) {
      if ((step.onFailure ?? ConvertFailure.Error) === ConvertFailure.Error) throw e;
      if (step.onFailure === ConvertFailure.Null) FieldPath.set(v, step.field, null);
    }
    return event;
  }
}

class DropIfHandler extends StepHandler {
  readonly kind = TransformStepKind.DropIf;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent | null {
    if (!step.condition) return event;
    return FilterEvaluator.evaluate(event, step.condition).pass ? null : event;
  }
}

class JsHandler extends StepHandler {
  readonly kind = TransformStepKind.Js;
  apply(event: KafkaEvent, step: TransformStep, logs: string[]): KafkaEvent | null {
    const compiled = JsSandbox.compile(step.code);
    if (!compiled.fn) throw new Error(compiled.error);
    const result = compiled.fn(event, JsSandbox.context(logs));
    return result === undefined ? event : (result as KafkaEvent | null);
  }
}

/** Registry of step handlers keyed by kind; add a handler here to support a new step. */
/** Visual Mapper: sources read from the input, then removals and assignments on the started output. */
class MapHandler extends StepHandler {
  readonly kind = TransformStepKind.Map;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const entries = step.mapEntries ?? [];
    const values = entries.map((e) => (e.remove ? undefined : MockValueSource.read(event, e.sourceKind, e.argument)));
    const out: Payload = step.mapStart === MapStart.Empty ? {} : this.payload(event);
    entries.filter((e) => e.remove).forEach((e) => FieldPath.remove(out, e.path));
    entries.forEach((e, i) => {
      if (!e.remove && values[i] !== undefined) FieldPath.set(out, e.path, structuredClone(values[i]));
    });
    event.value = out as KafkaEvent['value'];
    return event;
  }
}

/** Inline lookup tables run in the browser; library tables pass through; lookup sources apply the Studio mock (else pass through). */
class LookupHandler extends StepHandler {
  readonly kind = TransformStepKind.Lookup;
  apply(event: KafkaEvent, step: TransformStep, logs: string[]): KafkaEvent | null {
    if (step.lookupMode === LookupMode.Source) return step.lookupSource ? LookupAnswerApplier.apply(event, step.lookupSource, logs) : event;
    const v = this.payload(event);
    const key = FieldPath.get(v, step.field);
    if (key === undefined || step.lookupMode === LookupMode.Table) return event;
    const row = (step.lookupRows ?? []).find((r) => r.key === String(key));
    const target = step.argument || step.field;
    if (row) FieldPath.set(v, target, row.value);
    else if (step.fallback?.trim()) FieldPath.set(v, target, StepConstant.parse(step.fallback));
    return event;
  }
}

/** The in-browser runner keeps one event: split array continues with the first element and logs the fan-out. */
class SplitArrayHandler extends StepHandler {
  readonly kind = TransformStepKind.SplitArray;
  static readonly note = (count: number) => `split into ${count} events; the in-browser preview follows the first`;
  apply(event: KafkaEvent, step: TransformStep, logs: string[]): KafkaEvent | null {
    const items = FieldPath.get(this.payload(event), step.field);
    if (!Array.isArray(items)) return event;
    if (!items.length) return null;
    logs.push(SplitArrayHandler.note(items.length));
    FieldPath.set(this.payload(event), step.argument || step.field, structuredClone(items[0]));
    return event;
  }
}

class WrapHandler extends StepHandler {
  readonly kind = TransformStepKind.Wrap;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const wrapped: Payload = {};
    FieldPath.set(wrapped, step.field, this.payload(event));
    event.value = wrapped as KafkaEvent['value'];
    return event;
  }
}

class UnwrapHandler extends StepHandler {
  readonly kind = TransformStepKind.Unwrap;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const inner = FieldPath.get(this.payload(event), step.field);
    if (inner !== undefined) event.value = inner as KafkaEvent['value'];
    return event;
  }
}

/** Header text of a payload value: strings as-is, other JSON serialised. */
class HeaderText {
  static of(value: unknown): string {
    return typeof value === 'string' ? value : JSON.stringify(value);
  }
}

class HeaderAddHandler extends StepHandler {
  readonly kind = TransformStepKind.HeaderAdd;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const value = MockValueSource.read(event, step.sourceKind, step.argument);
    if (step.header && value !== undefined) event.headers = { ...event.headers, [step.header]: HeaderText.of(value) };
    return event;
  }
}

class HeaderRemoveHandler extends StepHandler {
  readonly kind = TransformStepKind.HeaderRemove;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const headers = { ...event.headers };
    delete headers[step.header ?? ''];
    event.headers = headers;
    return event;
  }
}

class HeaderRenameHandler extends StepHandler {
  readonly kind = TransformStepKind.HeaderRename;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const name = step.header ?? '';
    if (!(name in event.headers) || !step.argument) return event;
    const { [name]: value, ...rest } = event.headers;
    event.headers = { ...rest, [step.argument]: value };
    return event;
  }
}

class HeaderToPayloadHandler extends StepHandler {
  readonly kind = TransformStepKind.HeaderToPayload;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const value = event.headers[step.header ?? ''];
    if (value !== undefined && step.field) FieldPath.set(this.payload(event), step.field, value);
    return event;
  }
}

class HeaderFromPayloadHandler extends StepHandler {
  readonly kind = TransformStepKind.HeaderFromPayload;
  apply(event: KafkaEvent, step: TransformStep): KafkaEvent {
    const value = FieldPath.get(this.payload(event), step.field);
    if (value !== undefined && step.header) event.headers = { ...event.headers, [step.header]: HeaderText.of(value) };
    return event;
  }
}

/** Library steps expand on the server; the in-browser runner passes the event through and logs it. */
class LibraryHandler extends StepHandler {
  readonly kind = TransformStepKind.Library;
  static readonly note = (ref: string | undefined, version: number | undefined) => `library ${ref ?? '?'} v${version ?? '?'} runs on the preview service`;
  apply(event: KafkaEvent, step: TransformStep, logs: string[]): KafkaEvent {
    logs.push(LibraryHandler.note(step.libraryRef, step.libraryVersion));
    return event;
  }
}

export class StepHandlerRegistry {
  private static readonly handlers = new Map<TransformStepKind, StepHandler>(
    [
      new RenameHandler(),
      new MaskHandler(),
      new HashHandler(),
      new RemoveHandler(),
      new KeepOnlyHandler(),
      new CopyHandler(),
      new MoveHandler(),
      new ConvertTypeHandler(),
      new SetValueHandler(),
      new DefaultIfMissingHandler(),
      new DropIfHandler(),
      new SetKeyHandler(),
      new MapHandler(),
      new LookupHandler(),
      new SplitArrayHandler(),
      new WrapHandler(),
      new UnwrapHandler(),
      new HeaderAddHandler(),
      new HeaderRemoveHandler(),
      new HeaderRenameHandler(),
      new HeaderToPayloadHandler(),
      new HeaderFromPayloadHandler(),
      new LibraryHandler(),
      new JsHandler(),
    ].map((h) => [h.kind, h]),
  );

  static get(kind: TransformStepKind): StepHandler {
    return StepHandlerRegistry.handlers.get(kind) as StepHandler;
  }
}
