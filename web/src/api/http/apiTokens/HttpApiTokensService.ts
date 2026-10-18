import type { ApiToken, ApiTokenDraft, CreatedApiToken } from '../../../domain';
import { ApiTokensService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { ApiTokenEndpoint, ApiTokenMapper, type ApiTokenCreatedDto, type ApiTokenListDto } from './ApiTokenDtos';

/** Personal API tokens over `GET/POST /auth/tokens` and `DELETE /auth/tokens/{id}`. */
export class HttpApiTokensService extends ApiTokensService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<ApiToken[]> {
    const page = await this.context.api.get<ApiTokenListDto>(ApiTokenEndpoint.tokens);
    return page.items.map((dto) => ApiTokenMapper.token(dto));
  }

  async create(draft: ApiTokenDraft): Promise<CreatedApiToken> {
    const dto = await this.context.api.post<ApiTokenCreatedDto>(ApiTokenEndpoint.tokens, ApiTokenMapper.request(draft));
    return ApiTokenMapper.created(dto);
  }

  async revoke(id: string): Promise<void> {
    await this.context.api.delete(ApiTokenEndpoint.token(id));
  }
}
