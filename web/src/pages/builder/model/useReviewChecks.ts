import { services } from '../../../api';
import { RouteStatus, ValidationMode, VersionRef, type RouteDiff, type RouteDraft, type ValidationResult } from '../../../domain';
import { useServiceQuery } from '../../../store';

/** Server-side review data: deploy-mode validation of the current draft and the diff active ↔ saved draft. */
export type ReviewChecks = {
  validation: ValidationResult | null;
  validationError: Error | undefined;
  diff: RouteDiff | null;
  diffError: Error | undefined;
  /** True while the draft is unsaved (the diff compares the stored draft). */
  diffStale: boolean;
  firstDeploy: boolean;
};

/** Loads validation (re-run on every draft change) and the active ↔ draft diff (re-run after saves). */
export function useReviewChecks(draft: RouteDraft, dirty: boolean): ReviewChecks {
  const routeId = draft.routeId;
  const firstDeploy = !routeId || draft.status === RouteStatus.Draft;
  const validation = useServiceQuery(
    () => (routeId ? services.routes.validate(routeId, draft, ValidationMode.Deploy) : Promise.resolve(null)),
    [routeId, JSON.stringify(draft)],
  );
  const diff = useServiceQuery(
    () => (routeId && !firstDeploy && !dirty ? services.routes.diff(routeId, VersionRef.Active, VersionRef.Draft) : Promise.resolve(null)),
    [routeId, firstDeploy, dirty],
  );
  return {
    validation: validation.data ?? null,
    validationError: validation.error,
    diff: diff.data ?? null,
    diffError: diff.error,
    diffStale: dirty,
    firstDeploy,
  };
}
