import { Card, CardPadding } from '../../../../components/ui';
import { IssueSeverity, type BuilderStep, type Issue } from '../../../../domain';
import { IssueLocator } from '../../model/IssueLocator';
import type { ValidationCheck } from '../../model/ReviewModel';
import styles from './ValidationCard.module.css';

export type ValidationCardProps = {
  issues: Issue[] | null;
  error: Error | undefined;
  checks: ValidationCheck[];
  onOpen: (step: BuilderStep) => void;
};

/** Server validation (errors block deploy, warnings do not), each linked to its step and field, plus sample checks. */
export function ValidationCard({ issues, error, checks, onOpen }: ValidationCardProps) {
  return (
    <Card padding={CardPadding.Md} className={styles.card}>
      <div className={styles.title}>Validation</div>
      {issues === null && !error && <div className={styles.muted}>Save the draft to run the server checks.</div>}
      {error && <div className={styles.check} data-ok="false"><span className={styles.icon}>⚠</span><span>Validation unavailable: {error.message}</span></div>}
      {issues?.length === 0 && <div className={styles.check} data-ok="true"><span className={styles.icon}>✓</span><span>Configuration is valid</span></div>}
      {issues?.map((issue, i) => <IssueRow key={`${issue.path}-${i}`} issue={issue} onOpen={onOpen} />)}
      {checks.map((c) => (
        <div key={c.text} className={styles.check} data-ok={c.ok}>
          <span className={styles.icon}>{c.ok ? '✓' : '⚠'}</span>
          <span>{c.text}</span>
        </div>
      ))}
    </Card>
  );
}

/** One issue: severity icon, message and a link to the step + field it points at. */
function IssueRow({ issue, onOpen }: { issue: Issue; onOpen: (step: BuilderStep) => void }) {
  const target = IssueLocator.locate(issue);
  const error = issue.severity === IssueSeverity.Error;
  return (
    <button type="button" className={styles.issue} data-severity={issue.severity} onClick={() => onOpen(target.step)}>
      <span className={styles.icon}>{error ? '✕' : '⚠'}</span>
      <span className={styles.message}>{issue.message}</span>
      <span className={styles.where}>
        {target.step}
        {target.field && ` › ${target.field}`}
      </span>
    </button>
  );
}
