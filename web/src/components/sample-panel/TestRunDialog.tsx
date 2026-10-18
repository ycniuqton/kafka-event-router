import { Link } from 'react-router-dom';
import type { RunResult } from '../../domain';
import { AppPaths } from '../../app/paths';
import { PreviewPhase } from '../../store';
import { Button, ButtonVariant, ModalWidth, Modal } from '../ui';
import { EventStatusBadge } from './EventStatusBadge';
import { TestRunReport, TestTopicReport } from './TestRunReport';
import styles from './TestRunDialog.module.css';

export type TestRunDialogProps = {
  result: RunResult | null;
  onClose: () => void;
  /** Re-runs the test and also writes passed outputs to a temporary test topic (FR-22); omit to hide the option. */
  onWriteToTopic?: () => void;
  testPhase?: PreviewPhase;
};

/** Test results: counts, output validation, the failing events and the optional test-topic write. */
export function TestRunDialog({ result, onClose, onWriteToTopic, testPhase }: TestRunDialogProps) {
  if (!result) return null;
  const report = new TestRunReport(result);
  return (
    <Modal open onClose={onClose} title="Test run" subtitle={report.summary} width={ModalWidth.Md}
      footer={<Button variant={ButtonVariant.Primary} onClick={onClose}>Close</Button>}>
      <div className={styles.body} data-testid="test-run-report">
        <div className={styles.line} data-ok={report.ok || undefined}>
          {report.ok ? 'All outputs are valid for the destination.' : `${report.invalidOutputs} invalid outputs · ${report.failures.length} failing events`}
        </div>
        <div className={styles.meta}>{report.outputs} outputs encoded</div>
        {report.incomplete && <div className={styles.warn}>{report.incomplete}</div>}
        <TestTopicSection result={result} onWriteToTopic={onWriteToTopic} writing={testPhase === PreviewPhase.Loading} />
        {report.failures.map((f) => (
          <div key={f.seq} className={styles.failure}>
            <EventStatusBadge state={f.status} />
            <span className={styles.seq}>#{f.seq + 1}</span>
            <span className={styles.step}>{f.step}</span>
            <span className={styles.message}>{f.message}</span>
          </div>
        ))}
      </div>
    </Modal>
  );
}

type TestTopicSectionProps = { result: RunResult; onWriteToTopic?: () => void; writing: boolean };

/** Written test topic with a link to the connections screen, or the "Also write to test topic" action. */
function TestTopicSection({ result, onWriteToTopic, writing }: TestTopicSectionProps) {
  if (result.testTopic) {
    const topic = new TestTopicReport(result.testTopic);
    return (
      <div className={styles.topic} data-testid="test-topic">
        <span>{topic.line}</span>
        <Link className={styles.link} to={AppPaths.connections()}>
          {TestTopicReport.linkLabel}
        </Link>
      </div>
    );
  }
  if (!onWriteToTopic) return null;
  return (
    <div className={styles.topic}>
      <Button disabled={writing} onClick={onWriteToTopic} data-testid="write-test-topic">
        {writing ? TestTopicReport.writing : TestTopicReport.action}
      </Button>
      <span className={styles.meta}>{TestTopicReport.hint}</span>
    </div>
  );
}
