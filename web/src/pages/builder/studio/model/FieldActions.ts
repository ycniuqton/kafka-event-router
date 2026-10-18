import { MaskMode, StepFactory, TransformStepKind, type JsonLine, type TransformStep } from '../../../../domain';

/** A click-to-transform action on an Input field; builds the matching recipe step. */
export abstract class FieldAction {
  abstract readonly id: string;
  abstract readonly label: string;
  abstract build(line: JsonLine): TransformStep;

  /** Change label of the added step (design: "Added Mask email"). */
  changeLabel(path: string): string {
    return `Added ${this.changeVerb} ${path}`;
  }

  protected get changeVerb(): string {
    return this.label;
  }
}

class MaskFieldAction extends FieldAction {
  readonly id = 'mask';
  readonly label = 'Mask';
  static readonly emailMarker = '@';
  build(line: JsonLine): TransformStep {
    const isEmail = String(line.value).includes(MaskFieldAction.emailMarker);
    return StepFactory.create(TransformStepKind.Mask, { field: line.path, maskMode: isEmail ? MaskMode.Email : MaskMode.Last4 });
  }
}

class RenameFieldAction extends FieldAction {
  readonly id = 'rename';
  readonly label = 'Rename';
  static readonly snakeSegment = /_(\w)/g;
  build(line: JsonLine): TransformStep {
    const camel = line.key.replace(RenameFieldAction.snakeSegment, (_, c: string) => c.toUpperCase());
    return StepFactory.create(TransformStepKind.Rename, { field: line.path, argument: camel });
  }
}

class RemoveFieldAction extends FieldAction {
  readonly id = 'remove';
  readonly label = 'Remove';
  build(line: JsonLine): TransformStep {
    return StepFactory.create(TransformStepKind.Remove, { field: line.path });
  }
}

class UseAsKeyFieldAction extends FieldAction {
  readonly id = 'key';
  readonly label = 'Use as key';
  protected get changeVerb(): string {
    return 'Set key from';
  }
  build(line: JsonLine): TransformStep {
    return StepFactory.create(TransformStepKind.SetKey, { field: line.path });
  }
}

/** Click-to-transform actions in the design's order: Mask · Rename · Remove · Use as key. */
export class FieldActionCatalog {
  static readonly all: FieldAction[] = [
    new MaskFieldAction(),
    new RenameFieldAction(),
    new RemoveFieldAction(),
    new UseAsKeyFieldAction(),
  ];
}

