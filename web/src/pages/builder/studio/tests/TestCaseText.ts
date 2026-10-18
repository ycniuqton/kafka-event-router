/** Texts of the Studio test cases panel. */
export class TestCaseText {
  static readonly title = 'Test cases';
  static readonly pin = '+ Pin current event';
  static readonly pinTitle = 'Save the current event and its current output as a test case';
  static readonly run = 'Run';
  static readonly running = 'Running…';
  static readonly empty = 'Pin an event to lock its output. Every edit re-runs the cases; failing cases block deploy.';
  static readonly unsaved = 'Save the route once to pin test cases.';
  static readonly noRun = 'Run the preview first: the pinned output comes from it.';
  static readonly pending = 'Not run';
  static readonly diff = 'Diff';
  static readonly hideDiff = 'Hide diff';
  static readonly acceptActual = 'Accept actual';
  static readonly acceptTitle = 'Lock the actual result as the expected one';
  static readonly delete = 'Delete test case';
  static readonly rename = 'Test case name';
  static readonly expects = 'expects';
  static readonly outcome = 'outcome';
  static readonly pinned = (name: string) => `Pinned “${name}” as a test case`;
  static summary(passed: number, failed: number): string {
    return failed ? `${passed} passing · ${failed} failing` : `${passed} passing`;
  }
}
