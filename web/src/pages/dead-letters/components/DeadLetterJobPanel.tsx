import type { CSSProperties } from 'react';
import { Button } from '../../../components/ui';
import { DeadLetterJobStatus, DeadLetterJobs, type DeadLetterJob } from '../../../domain';
import { actions, useDeadLetterJobs } from '../../../store';
import styles from './DeadLetterJobPanel.module.css';

export type DeadLetterJobPanelProps = { routeId?: string };

/** Progress of background jobs of this session (one route, or all on the global screen): bar, cancel, dismiss. */
export function DeadLetterJobPanel({ routeId }: DeadLetterJobPanelProps) {
  const jobs = useDeadLetterJobs().filter((j) => !routeId || j.routeId === routeId);
  if (!jobs.length) return null;
  return (
    <div className={styles.panel} role="status">
      {jobs.map((job) => (
        <JobRow key={job.id} job={job} />
      ))}
    </div>
  );
}

function JobRow({ job }: { job: DeadLetterJob }) {
  const done = DeadLetterJobs.done(job);
  const running = job.status === DeadLetterJobStatus.Running || job.status === DeadLetterJobStatus.Queued;
  return (
    <div className={styles.row} data-status={job.status}>
      <div className={styles.text}>
        <span>{DeadLetterJobs.label(job)}</span>
        {job.error && <span className={styles.error}>{job.error}</span>}
      </div>
      <div
        className={styles.track}
        role="progressbar"
        aria-label="Job progress"
        aria-valuenow={DeadLetterJobs.percent(job)}
        aria-valuemin={0}
        aria-valuemax={DeadLetterJobs.percentScale}
        style={{ '--p': `${DeadLetterJobs.percent(job)}%` } as CSSProperties}
      >
        <div className={styles.fill} />
      </div>
      {running && (
        <Button size="sm" variant="ghost" onClick={() => void actions.deadLetters.cancelJob(job.id)}>
          Cancel
        </Button>
      )}
      {done && (
        <Button size="sm" variant="ghost" onClick={() => actions.deadLetters.dismissJob(job.id)}>
          Dismiss
        </Button>
      )}
    </div>
  );
}
