import { Can, DeployModeResolver, PendingApprovalBanner, useDeployMode } from '../../../../access';
import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { DeployMode, Permission, RouteDiffRules } from '../../../../domain';
import type { StepEditorProps } from '../StepEditorProps';
import { ReviewModel } from '../../model/ReviewModel';
import { useReviewChecks } from '../../model/useReviewChecks';
import { StepHeading } from '../StepHeading';
import { ChangesCard } from './ChangesCard';
import { DryRunCard } from './DryRunCard';
import { ReviewSentenceCard } from './ReviewSentenceCard';
import { ValidationCard } from './ValidationCard';
import styles from './ReviewStep.module.css';

/** Review & deploy (spec §7.4): route as a sentence, server validation, diff vs running version, dry run, then Deploy. */
export function ReviewStep({ ctx }: StepEditorProps) {
  const { draft, preview, connections } = ctx;
  const checks = useReviewChecks(draft, ctx.dirty.size > 0);
  const errors = RouteDiffRules.errors(checks.validation?.issues ?? []);
  const mode = useDeployMode(draft);
  const canDeploy = ReviewModel.canDeploy(draft) && !errors.length && mode !== DeployMode.Disabled;
  const blocked = errors.length ? `Fix ${errors.length} validation error${errors.length > 1 ? 's' : ''} first` : 'Pick a source topic and a destination topic first';
  return (
    <div className={styles.step}>
      <StepHeading title="Review & deploy" subtitle="Nothing goes live unvalidated." />
      {draft.routeId && <PendingApprovalBanner routeId={draft.routeId} />}
      <ReviewSentenceCard sentence={ReviewModel.sentence(draft, connections)} />
      <div className={styles.grid}>
        <ValidationCard issues={checks.validation?.issues ?? null} error={checks.validationError} checks={ReviewModel.checks(preview)} onOpen={ctx.goTo} />
        <ChangesCard checks={checks} changes={draft.changes} onSave={ctx.save} />
      </div>
      <DryRunCard lines={ReviewModel.dryRun(preview)} />
      <div className={styles.actions}>
        <Button size={ButtonSize.Lg} className={styles.tall} onClick={ctx.runTest}>
          Run test
        </Button>
        <Can permission={Permission.RouteDeployNonProd}>
          <Button
            variant={ButtonVariant.Primary}
            size={ButtonSize.Lg}
            className={`${styles.tall} ${styles.deploy}`}
            disabled={!canDeploy}
            title={canDeploy ? undefined : blocked}
            onClick={ctx.openDeploy}
          >
            {DeployModeResolver.label(mode, ReviewModel.deployTitle(draft))}
          </Button>
        </Can>
      </div>
    </div>
  );
}
