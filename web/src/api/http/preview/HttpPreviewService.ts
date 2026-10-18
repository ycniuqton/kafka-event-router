import type { FieldSchema, LoadSamplesRequest, PreviewFailure, RunRequest, RunResult, SampleLoad, SamplePage, SampleQuery, ValueFormat } from '../../../domain/preview';
import { PreviewService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpPreviewClient } from './HttpPreviewClient';
import { PreviewFailureClassifier } from './PreviewFailureClassifier';

/** `services.preview` over `/api/v1/preview/*` (no mock fallback: failures reach the panel). */
export class HttpPreviewService extends PreviewService {
  private readonly client: HttpPreviewClient;

  constructor(context: HttpContext) {
    super();
    this.client = new HttpPreviewClient(context);
  }

  loadSamples(request: LoadSamplesRequest): Promise<SampleLoad> {
    return this.client.loadSamples(request);
  }

  listEvents(sessionId: string, query?: SampleQuery): Promise<SamplePage> {
    return this.client.listEvents(sessionId, query);
  }

  schema(sessionId: string): Promise<FieldSchema> {
    return this.client.schema(sessionId);
  }

  run(request: RunRequest, signal?: AbortSignal): Promise<RunResult> {
    return this.client.run(request, signal);
  }

  testRun(request: RunRequest, signal?: AbortSignal): Promise<RunResult> {
    return this.client.testRun(request, signal);
  }

  reformat(sessionId: string, format: ValueFormat): Promise<SampleLoad> {
    return this.client.reformat(sessionId, format);
  }

  close(sessionId: string): Promise<void> {
    return this.client.close(sessionId);
  }

  failure(error: unknown): PreviewFailure {
    return PreviewFailureClassifier.classify(error);
  }
}
