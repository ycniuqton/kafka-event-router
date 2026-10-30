import { LookupKindSettingsOf, LookupSourceInUseError, type LookupSecrets, type LookupSource, type LookupSourceDraft, type LookupTestResult } from '../../../domain';
import { LookupSourcesService } from '../../contracts';
import { ApiError } from '../core/ApiError';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import { HttpMethod } from '../core/HttpConstants';
import type { HttpContext } from '../core/HttpContext';
import {
  LookupSourceEndpoint,
  LookupSourceProblem,
  type LookupSourceDto,
  type LookupSourceInputDto,
  type LookupSourceProblemExtras,
  type LookupTestResultDto,
} from './LookupSourceDtos';

/** Draft → request bodies: only the draft kind's settings; secrets only when typed (blank = keep stored). */
export class LookupSourceInputMapper {
  static input(d: LookupSourceDraft): LookupSourceInputDto {
    const input: LookupSourceInputDto = {
      name: d.name.trim(), description: d.description, kind: d.kind, envTag: d.envTag, sensitive: d.sensitive,
      ...LookupKindSettingsOf.draft(d), limits: d.limits,
    };
    if (d.ownerTeamId) input.owner = { teamId: d.ownerTeamId };
    const secrets = LookupSourceInputMapper.secrets(d.secrets);
    if (secrets) input.secrets = secrets;
    return input;
  }

  private static secrets(s: LookupSecrets): LookupSecrets | null {
    const typed = Object.entries(s).filter(([, v]) => !!v?.trim());
    return typed.length ? Object.fromEntries(typed) : null;
  }
}

/** In-use problems become `LookupSourceInUseError`; other errors pass through. */
export class LookupSourceProblemTranslator {
  static translate(error: unknown): unknown {
    if (!(error instanceof ApiError) || !error.type?.endsWith(LookupSourceProblem.InUseSuffix)) return error;
    const extras = error.problem as unknown as LookupSourceProblemExtras;
    return new LookupSourceInUseError(error.message, extras.routes ?? []);
  }
}

/** `services.lookupSources` over `/api/v1/lookup-sources`: cursor-paged list, merge-patch edits with If-Match, sync tests. */
export class HttpLookupSourcesService extends LookupSourcesService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<LookupSource[]> {
    const pages = new CursorIterator<LookupSourceDto>(this.context.api, LookupSourceEndpoint.collection, { limit: PageLimit.Max });
    return (await pages.all()).map((dto) => this.remember(dto));
  }

  async get(id: string): Promise<LookupSource> {
    const res = await this.context.api.send<LookupSourceDto>({ method: HttpMethod.Get, path: LookupSourceEndpoint.item(id) });
    this.context.etags.remember(LookupSourceEndpoint.item(id), res.etag);
    return res.data;
  }

  async create(draft: LookupSourceDraft): Promise<LookupSource> {
    const res = await this.context.api.send<LookupSourceDto>({ method: HttpMethod.Post, path: LookupSourceEndpoint.collection, body: LookupSourceInputMapper.input(draft) });
    this.context.etags.remember(LookupSourceEndpoint.item(res.data.id), res.etag);
    return res.data;
  }

  async update(id: string, draft: LookupSourceDraft): Promise<LookupSource> {
    const path = LookupSourceEndpoint.item(id);
    const ifMatch = this.context.etags.ifMatch(path) ?? `"${(await this.context.api.get<LookupSourceDto>(path)).version}"`;
    const res = await this.context.api.send<LookupSourceDto>({ method: HttpMethod.Patch, path, ifMatch, body: LookupSourceInputMapper.input(draft) });
    this.context.etags.remember(path, res.etag);
    return res.data;
  }

  async remove(id: string): Promise<void> {
    try {
      await this.context.api.delete(LookupSourceEndpoint.item(id));
    } catch (error) {
      throw LookupSourceProblemTranslator.translate(error);
    }
    this.context.etags.forget(LookupSourceEndpoint.item(id));
  }

  async test(id: string): Promise<LookupTestResult> {
    return this.context.api.post<LookupTestResultDto>(LookupSourceEndpoint.test(id));
  }

  async testDraft(draft: LookupSourceDraft): Promise<LookupTestResult> {
    return this.context.api.post<LookupTestResultDto>(LookupSourceEndpoint.draftTest, LookupSourceInputMapper.input(draft));
  }

  private remember(dto: LookupSourceDto): LookupSource {
    this.context.etags.remember(LookupSourceEndpoint.item(dto.id), `"${dto.version}"`);
    return dto;
  }
}
