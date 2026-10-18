/** Texts of the export-to-JS flow. */
export class ExportText {
  static readonly start = 'Export to JS';
  static readonly startTitle = 'Convert selected recipe steps into one equivalent JavaScript step';
  static readonly prompt = 'Select the recipe steps to convert';
  static readonly convert = 'Convert to JS';
  static readonly converting = 'Converting…';
  static readonly cancel = 'Cancel';
  static selected(n: number): string {
    return `${n} selected`;
  }
}
