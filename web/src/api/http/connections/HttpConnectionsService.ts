import {
  ConnectionInUseError,
  type Connection,
  type ConnectionDraft,
  type ConnectionTestOptions,
  type ConnectionTestRun,
  type TopicInfo,
  type TopicQuery,
  type TopicSettings,
} from '../../../domain';
import { ConnectionsService } from '../../contracts';
import { ApiError } from '../core/ApiError';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import { HttpMethod } from '../core/HttpConstants';
import type { HttpContext } from '../core/HttpContext';
import {
  ConnectionEndpoint,
  ConnectionProblemType,
  TopicQueryKey,
  type ConnectionDto,
  type ConnectionProblemExtras,
  type CreatedTopicDto,
  type TestLineDto,
  type TopicDto,
  type TopicInputDto,
} from './ConnectionDtos';
import { ConnectionMapper } from './ConnectionMapper';
import { ConnectionTestAssembler } from './ConnectionTestAssembler';
import { NdjsonStreamReader } from './NdjsonStreamReader';

/** Thrown when a test stream ends without its result line (connection dropped mid-run). */
export class TestStreamInterrupted extends Error {
  static readonly text = 'The connection test was interrupted before it finished. Test again.';

  constructor() {
    super(TestStreamInterrupted.text);
    this.name = 'TestStreamInterrupted';
  }
}

/** Turns connection problems into domain errors: in-use → `ConnectionInUseError`, cluster errors keep their hint. */
export class ConnectionProblemTranslator {
  static readonly separator = ' ';

  static translate(error: unknown): unknown {
    if (!(error instanceof ApiError) || !error.problem) return error;
    const extras = error.problem as unknown as ConnectionProblemExtras;
    if (error.type === ConnectionProblemType.InUse) return new ConnectionInUseError(error.message, extras.routes ?? []);
    if (!extras.hint) return error;
    const enriched = new ApiError(error.kind, error.status, error.problem, [error.message, extras.hint].join(ConnectionProblemTranslator.separator));
    return enriched;
  }
}

/**
 * `services.connections` over `/api/v1/connections`: cursor-paged list, write-only secrets,
 * merge-patch edits with If-Match, the live test checklist streamed as NDJSON, topics.
 */
export class HttpConnectionsService extends ConnectionsService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<Connection[]> {
    const pages = new CursorIterator<ConnectionDto>(this.context.api, ConnectionEndpoint.collection, { limit: PageLimit.Max });
    return (await pages.all()).map((dto) => this.remember(dto));
  }

  async get(id: string): Promise<Connection> {
    const res = await this.context.api.send<ConnectionDto>({ method: HttpMethod.Get, path: ConnectionEndpoint.item(id) });
    this.context.etags.remember(ConnectionEndpoint.item(id), res.etag);
    return ConnectionMapper.connection(res.data);
  }

  async create(draft: ConnectionDraft): Promise<Connection> {
    const res = await this.context.api.send<ConnectionDto>({
      method: HttpMethod.Post,
      path: ConnectionEndpoint.collection,
      body: ConnectionMapper.input(draft),
    });
    this.context.etags.remember(ConnectionEndpoint.item(res.data.id), res.etag);
    return ConnectionMapper.connection(res.data);
  }

  async update(id: string, draft: ConnectionDraft): Promise<Connection> {
    const path = ConnectionEndpoint.item(id);
    const ifMatch = this.context.etags.ifMatch(path);
    const current = await this.context.api.get<ConnectionDto>(path);
    const res = await this.context.api.send<ConnectionDto>({
      method: HttpMethod.Patch,
      path,
      ifMatch: ifMatch ?? `"${current.version}"`,
      body: ConnectionMapper.patch(draft, current.properties ?? {}),
    });
    this.context.etags.remember(path, res.etag);
    return ConnectionMapper.connection(res.data);
  }

  async remove(id: string): Promise<void> {
    try {
      await this.context.api.delete(ConnectionEndpoint.item(id));
    } catch (error) {
      throw ConnectionProblemTranslator.translate(error);
    }
    this.context.etags.forget(ConnectionEndpoint.item(id));
  }

  test(id: string, onUpdate: (run: ConnectionTestRun) => void, options: ConnectionTestOptions = {}): Promise<ConnectionTestRun> {
    return this.stream(id, ConnectionEndpoint.test(id), options, onUpdate);
  }

  testDraft(
    draft: ConnectionDraft,
    connectionId: string | null,
    onUpdate: (run: ConnectionTestRun) => void,
    options: ConnectionTestOptions = {},
  ): Promise<ConnectionTestRun> {
    const body = { ...ConnectionMapper.input(draft), ...options, ...(connectionId ? { connectionId } : {}) };
    return this.stream(connectionId ?? draft.name, ConnectionEndpoint.draftTest, body, onUpdate);
  }

  async listTopics(id: string, query: TopicQuery = {}): Promise<TopicInfo[]> {
    const params = { [TopicQueryKey.Text]: query.q || undefined, [TopicQueryKey.IncludeInternal]: query.includeInternal || undefined };
    try {
      const res = await this.context.api.get<{ items: TopicDto[] }>(ConnectionEndpoint.topics(id), params);
      return res.items.map((t) => ConnectionMapper.topic(t));
    } catch (error) {
      throw ConnectionProblemTranslator.translate(error);
    }
  }

  async createTopic(id: string, name: string, settings: TopicSettings = {}): Promise<TopicInfo> {
    const body: TopicInputDto = { name, ...settings };
    try {
      return ConnectionMapper.createdTopic(await this.context.api.post<CreatedTopicDto>(ConnectionEndpoint.topics(id), body));
    } catch (error) {
      throw ConnectionProblemTranslator.translate(error);
    }
  }

  private async stream(key: string, path: string, body: unknown, onUpdate: (run: ConnectionTestRun) => void): Promise<ConnectionTestRun> {
    const response = await this.context.api.raw({ method: HttpMethod.Post, path, body });
    const assembler = new ConnectionTestAssembler(key);
    let last = assembler.snapshot();
    onUpdate(last);
    if (!response.body) throw new TestStreamInterrupted();
    for await (const line of new NdjsonStreamReader<TestLineDto>(response.body)) {
      last = assembler.apply(line);
      onUpdate(last);
    }
    if (!last.finished) throw new TestStreamInterrupted();
    return last;
  }

  private remember(dto: ConnectionDto): Connection {
    this.context.etags.remember(ConnectionEndpoint.item(dto.id), `"${dto.version}"`);
    return ConnectionMapper.connection(dto);
  }
}
