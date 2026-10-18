import { ApiError } from '../core/ApiError';
import type { HttpContext } from '../core/HttpContext';
import {
  PreviewEndpoint,
  PreviewProblemType,
  PreviewQuery,
  type EventsQuery,
  type FieldSchemaDto,
  type LoadSamplesRequest,
  type LoadSamplesResponse,
  type RunRequestDto,
  type RunResultDto,
  type SamplesPageDto,
} from './PreviewDtos';

/**
 * REST client of live preview (`/api/v1/preview/*`): sample sessions, schema and traced runs.
 * Runs accept an AbortSignal so a newer draft cancels the stale run.
 */
export class HttpPreviewClient {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    this.context = context;
  }

  loadSamples(request: LoadSamplesRequest): Promise<LoadSamplesResponse> {
    return this.context.api.post<LoadSamplesResponse>(PreviewEndpoint.samples, request);
  }

  listEvents(sessionId: string, query: EventsQuery = {}): Promise<SamplesPageDto> {
    return this.context.api.get<SamplesPageDto>(PreviewEndpoint.events(sessionId), {
      [PreviewQuery.Cursor]: query.cursor,
      [PreviewQuery.Limit]: query.limit,
      [PreviewQuery.Text]: query.q,
    });
  }

  schema(sessionId: string): Promise<FieldSchemaDto> {
    return this.context.api.get<FieldSchemaDto>(PreviewEndpoint.schema(sessionId));
  }

  reformat(sessionId: string, format: string): Promise<LoadSamplesResponse> {
    return this.context.api.patch<LoadSamplesResponse>(PreviewEndpoint.session(sessionId), { format });
  }

  async close(sessionId: string): Promise<void> {
    await this.context.api.delete(PreviewEndpoint.session(sessionId), { silent: true });
  }

  run(request: RunRequestDto, signal?: AbortSignal): Promise<RunResultDto> {
    return this.context.api.post<RunResultDto>(PreviewEndpoint.run, request, { signal });
  }

  testRun(request: RunRequestDto, signal?: AbortSignal): Promise<RunResultDto> {
    return this.context.api.post<RunResultDto>(PreviewEndpoint.testRuns, request, { signal });
  }
}

/** Classifies preview failures the panel reacts to (silent reload, busy retry, inline config errors). */
export class PreviewErrors {
  static isSessionExpired(error: unknown): boolean {
    return PreviewErrors.hasType(error, PreviewProblemType.SessionExpired);
  }

  static isBusy(error: unknown): boolean {
    return PreviewErrors.hasType(error, PreviewProblemType.Busy);
  }

  static isInvalidConfig(error: unknown): boolean {
    return PreviewErrors.hasType(error, PreviewProblemType.InvalidConfig);
  }

  private static hasType(error: unknown, type: PreviewProblemType): boolean {
    return error instanceof ApiError && error.type === type;
  }
}
