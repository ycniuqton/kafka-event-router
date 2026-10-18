/** Texts of the Visual Mapper. */
export class MapperText {
  static readonly title = 'Visual mapper';
  static readonly start = 'Start from';
  static readonly pasteExample = 'Paste an example';
  static readonly input = 'Input fields';
  static readonly output = 'Output';
  static readonly dropHint = 'Drag input fields here, or add a field.';
  static readonly addField = '+ Add field';
  static readonly newField = 'field';
  static readonly fieldName = 'Output field name';
  static readonly removed = 'removed from the output';
  static readonly restore = 'Keep this field';
  static readonly delete = 'Delete';
  static readonly nest = 'Nest into an object';
  static readonly unnest = 'Move out of its object';
  static readonly groupName = 'Object name';
  static readonly groupPlaceholder = 'object name';
  static readonly enterKey = 'Enter';
  static readonly map = 'Map to output';
  static readonly removeInput = 'Remove from output';
  static readonly expand = 'Full screen';
  static readonly collapse = 'Exit full screen';
  static readonly noSample = 'Load a sample to see its fields.';
  static unmapped(n: number): string {
    return n === 1 ? '1 input field is not in the output' : `${n} input fields are not in the output`;
  }
}
