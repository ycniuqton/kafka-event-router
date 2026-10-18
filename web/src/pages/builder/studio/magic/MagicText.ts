/** Texts of the transform-by-example dialog. */
export class ByExampleText {
  static readonly open = 'By example';
  static readonly title = 'Transform by example';
  static readonly subtitle = 'Paste the output you want for the current event. Proposed steps are shown before anything changes.';
  static readonly label = 'Desired output (JSON value)';
  static readonly infer = 'Propose steps';
  static readonly seed = 'Start from the current event';
  static readonly working = 'Matching…';
  static readonly invalidJson = 'Paste valid JSON (an object).';
  static readonly noEvent = 'Load a sample event first.';
  static readonly indent = 2;
}

/** Texts of explanations. */
export class ExplainText {
  static readonly explainChain = 'Explain steps';
  static readonly hide = 'Hide explanation';
  static readonly loading = 'Explaining…';
}
