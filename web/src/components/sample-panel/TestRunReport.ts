import { EventStatus, SampleSummary, type RunResult, type TestTopic } from '../../domain';

/** One failing event of a test run. */
export type TestFailure = { seq: number; status: EventStatus; step: string; message: string };

/** What the Test dialog shows: summary, invalid outputs and the first failing events. */
export class TestRunReport {
  static readonly maxFailures = 20;

  readonly summary: string;
  readonly outputs: number;
  readonly invalidOutputs: number;
  readonly failures: TestFailure[];
  readonly incomplete: string | null;
  readonly ok: boolean;

  constructor(result: RunResult) {
    this.summary = SampleSummary.label(result.summary);
    const outputs = result.results.flatMap((r) => r.branches.flatMap((b) => b.outputs));
    this.outputs = outputs.length;
    this.invalidOutputs = outputs.filter((o) => !o.valid).length;
    this.failures = result.results
      .filter((r) => r.status === EventStatus.Error)
      .slice(0, TestRunReport.maxFailures)
      .map((r) => ({ seq: r.seq, status: r.status, step: r.error?.step ?? r.error?.errorType ?? '', message: r.error?.message ?? '' }));
    this.incomplete = SampleSummary.incompleteNotice(result.summary);
    this.ok = result.summary.errors === 0 && this.invalidOutputs === 0;
  }

  /** Toast text of the Test button (design `testRun`). */
  static toast(result: RunResult): string {
    const s = result.summary;
    return `Test run · ${s.loaded} events · ${s.passed} pass · ${s.filtered} filtered · ${s.errors} errors`;
  }
}

/** Texts of the test-topic write (FR-22: router.test.<route>.<timestamp>, deleted after 24 h). */
export class TestTopicReport {
  static readonly action = 'Also write to test topic';
  static readonly writing = 'Writing to test topic…';
  static readonly hint = 'Passed outputs go to a temporary router.test.* topic on the destination connection, never the real topic. Deleted after 24 h.';
  static readonly linkLabel = 'Open connections';
  private static readonly numbers = new Intl.NumberFormat('en-US');

  readonly line: string;

  constructor(topic: TestTopic) {
    const expires = new Date(topic.expiresAt).toUTCString();
    this.line = `Wrote ${TestTopicReport.numbers.format(topic.produced)} outputs to ${topic.topic} · deleted after ${expires}`;
  }

  /** Toast text after a test-topic write. */
  static toast(topic: TestTopic): string {
    return `Wrote ${TestTopicReport.numbers.format(topic.produced)} outputs to ${topic.topic}`;
  }
}
