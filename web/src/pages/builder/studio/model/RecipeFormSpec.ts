import { TransformStepKind, type TransformStep } from '../../../../domain';

/** Kinds of input a structure / Kafka recipe form is built from. */
export const RecipeInput = {
  /** An existing payload field, picked from the current sample. */
  FieldPick: 'fieldPick',
  /** A payload path typed by the user (may be new). */
  FieldPath: 'fieldPath',
  /** Optional target payload path (empty = in place). */
  Target: 'target',
  /** A record header name (suggested from the current sample). */
  Header: 'header',
  /** The new name of a renamed header. */
  NewHeader: 'newHeader',
  /** Fixed value, field, metadata or formula. */
  Source: 'source',
} as const;
export type RecipeInput = (typeof RecipeInput)[keyof typeof RecipeInput];

/** One input of a form: what it edits, its accessible label and its placeholder. */
export type RecipeFormInput = { input: RecipeInput; label: string; placeholder: string };

/** A form: its inputs in reading order and the operator shown between them. */
export type RecipeForm = { inputs: RecipeFormInput[]; op: string };

/** Operators shown between the inputs of a form row. */
export const RecipeFormOp = { To: '→', Gets: '←' } as const;

/** Step option written by each input (a value source edits `sourceKind` + `argument` itself). */
const RecipeInputOption: Record<Exclude<RecipeInput, typeof RecipeInput.Source>, 'field' | 'argument' | 'header'> = {
  [RecipeInput.FieldPick]: 'field',
  [RecipeInput.FieldPath]: 'field',
  [RecipeInput.Target]: 'argument',
  [RecipeInput.Header]: 'header',
  [RecipeInput.NewHeader]: 'argument',
};

const header: RecipeFormInput = { input: RecipeInput.Header, label: 'Header', placeholder: 'traceId' };

/** Structure and Kafka-header recipe forms (RecipeFormRegistry entries as data; one generic editor renders them). */
export class RecipeFormSpec {
  static readonly forms: Partial<Record<TransformStepKind, RecipeForm>> = {
    [TransformStepKind.SplitArray]: {
      op: RecipeFormOp.To,
      inputs: [
        { input: RecipeInput.FieldPick, label: 'Array field', placeholder: 'items' },
        { input: RecipeInput.Target, label: 'Item field', placeholder: 'same field' },
      ],
    },
    [TransformStepKind.Wrap]: { op: RecipeFormOp.To, inputs: [{ input: RecipeInput.FieldPath, label: 'Wrap in', placeholder: 'payload' }] },
    [TransformStepKind.Unwrap]: { op: RecipeFormOp.To, inputs: [{ input: RecipeInput.FieldPick, label: 'Unwrap field', placeholder: 'payload' }] },
    [TransformStepKind.HeaderAdd]: { op: RecipeFormOp.Gets, inputs: [header, { input: RecipeInput.Source, label: 'Header value', placeholder: '' }] },
    [TransformStepKind.HeaderRemove]: { op: RecipeFormOp.To, inputs: [header] },
    [TransformStepKind.HeaderRename]: {
      op: RecipeFormOp.To,
      inputs: [header, { input: RecipeInput.NewHeader, label: 'New header', placeholder: 'trace-id' }],
    },
    [TransformStepKind.HeaderToPayload]: {
      op: RecipeFormOp.To,
      inputs: [header, { input: RecipeInput.FieldPath, label: 'Target field', placeholder: 'meta.traceId' }],
    },
    [TransformStepKind.HeaderFromPayload]: {
      op: RecipeFormOp.To,
      inputs: [{ input: RecipeInput.FieldPick, label: 'Field', placeholder: 'orderId' }, header],
    },
  };

  static of(kind: TransformStepKind): RecipeForm {
    return RecipeFormSpec.forms[kind] ?? { op: RecipeFormOp.To, inputs: [] };
  }

  /** Current text of a (non-source) input. */
  static read(step: TransformStep, input: Exclude<RecipeInput, typeof RecipeInput.Source>): string {
    return step[RecipeInputOption[input]] ?? '';
  }

  /** Step patch of typing `text` into a (non-source) input. */
  static patch(input: Exclude<RecipeInput, typeof RecipeInput.Source>, text: string): Partial<TransformStep> {
    return { [RecipeInputOption[input]]: text };
  }
}
