import { describe, expect, it } from 'vitest';
import { PreviewFailureKind } from '../../../domain/preview';
import { ApiError } from '../core/ApiError';
import { PreviewFailureClassifier } from './PreviewFailureClassifier';
import { PreviewProblemType } from './PreviewDtos';

const problem = (type: string, status: number, path?: string) =>
  ApiError.fromProblem({ type, title: 't', status, detail: 'detail text', path } as Parameters<typeof ApiError.fromProblem>[0]);

describe('PreviewFailureClassifier', () => {
  it.each([
    [PreviewProblemType.SessionExpired, 410, PreviewFailureKind.SessionExpired],
    [PreviewProblemType.Busy, 429, PreviewFailureKind.Busy],
    [PreviewProblemType.InvalidConfig, 422, PreviewFailureKind.InvalidConfig],
    [PreviewProblemType.TopicNotFound, 404, PreviewFailureKind.TopicNotFound],
    [PreviewProblemType.SourceUnavailable, 502, PreviewFailureKind.SourceUnavailable],
    [PreviewProblemType.Forbidden, 403, PreviewFailureKind.Forbidden],
    ['https://eventrouter.dev/problems/other', 500, PreviewFailureKind.Other],
    ['https://eventrouter.dev/problems/rate-limited', 429, PreviewFailureKind.Busy],
  ])('%s → %s', (type, status, kind) => {
    expect(PreviewFailureClassifier.classify(problem(type, status)).kind).toBe(kind);
  });

  it('keeps the problem path and detail', () => {
    const f = PreviewFailureClassifier.classify(problem(PreviewProblemType.TopicNotFound, 404, '/topics/1'));
    expect(f).toEqual({ kind: PreviewFailureKind.TopicNotFound, message: 'detail text', path: '/topics/1' });
  });

  it('recognises aborted requests', () => {
    const abort = new DOMException('aborted', 'AbortError');
    expect(PreviewFailureClassifier.classify(abort).kind).toBe(PreviewFailureKind.Aborted);
  });
});
