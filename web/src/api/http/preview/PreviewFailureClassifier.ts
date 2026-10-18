import { PreviewFailureKind, type PreviewFailure } from '../../../domain/preview';
import { ApiError } from '../core/ApiError';
import { PreviewProblemType } from './PreviewDtos';

/** Maps preview problems (RFC 7807 `type`) and aborted requests to the panel's failure kinds. */
export class PreviewFailureClassifier {
  static readonly abortName = 'AbortError';

  private static readonly kinds: ReadonlyMap<string, PreviewFailureKind> = new Map<string, PreviewFailureKind>([
    [PreviewProblemType.SessionExpired, PreviewFailureKind.SessionExpired],
    [PreviewProblemType.InvalidConfig, PreviewFailureKind.InvalidConfig],
    [PreviewProblemType.Busy, PreviewFailureKind.Busy],
    [PreviewProblemType.SourceUnavailable, PreviewFailureKind.SourceUnavailable],
    [PreviewProblemType.TopicNotFound, PreviewFailureKind.TopicNotFound],
    [PreviewProblemType.InvalidUpload, PreviewFailureKind.InvalidUpload],
    [PreviewProblemType.Forbidden, PreviewFailureKind.Forbidden],
  ]);

  /** The gateway's per-user rate limit (429) is retried like a busy preview service. */
  static readonly rateLimited = 429;

  private static byStatus(status: number): PreviewFailureKind {
    return status === PreviewFailureClassifier.rateLimited ? PreviewFailureKind.Busy : PreviewFailureKind.Other;
  }

  static classify(error: unknown): PreviewFailure {
    if (error instanceof Error && error.name === PreviewFailureClassifier.abortName) {
      return { kind: PreviewFailureKind.Aborted, message: error.message };
    }
    if (!(error instanceof ApiError)) return { kind: PreviewFailureKind.Other, message: String((error as Error)?.message ?? error) };
    const kind = PreviewFailureClassifier.kinds.get(error.type ?? '') ?? PreviewFailureClassifier.byStatus(error.status);
    return { kind, message: error.message, path: error.problem?.path ?? undefined, fieldMessage: error.errors[0]?.message };
  }
}
