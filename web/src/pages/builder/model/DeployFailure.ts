import { ApiError, ProblemKind } from '../../../api/http/core';

/** What the deploy dialog shows after a failed deploy: the message and whether "Overwrite" applies. */
export type DeployFailureView = { message: string; conflict: boolean };

/** Classifies deploy errors (412 stale draft → reload/overwrite; 409 nothing to deploy; 422 validation issues). */
export class DeployFailure {
  static readonly conflictMessage = 'Someone else saved this route after you opened it. Reload to see their changes, or overwrite them with yours.';
  static readonly issuesSuffix = 'Open Review to see every issue.';

  static of(error: unknown): DeployFailureView {
    if (!(error instanceof ApiError)) return { message: (error as Error)?.message ?? String(error), conflict: false };
    if (error.problemKind === ProblemKind.PreconditionFailed) return { message: DeployFailure.conflictMessage, conflict: true };
    if (error.problemKind === ProblemKind.Validation) return { message: `${error.message} ${DeployFailure.issuesSuffix}`, conflict: false };
    return { message: error.message, conflict: false };
  }
}
