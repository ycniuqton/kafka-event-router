import type { ApiSchemas } from '../../generated';
import type { ApiToken, ApiTokenDraft, CreatedApiToken } from '../../../domain';

/** Wire shapes of `/auth/tokens` (generated from api/openapi/paths/auth.yaml). */
export type ApiTokenDto = ApiSchemas['ApiToken'];
export type ApiTokenListDto = ApiSchemas['ApiTokenList'];
export type ApiTokenCreateDto = ApiSchemas['ApiTokenCreate'];
export type ApiTokenCreatedDto = ApiSchemas['ApiTokenCreated'];

/** REST paths of API tokens (relative to `/api/v1`). */
export class ApiTokenEndpoint {
  static readonly tokens = '/auth/tokens';

  static token(id: string): string {
    return `${ApiTokenEndpoint.tokens}/${encodeURIComponent(id)}`;
  }
}

/** Maps between the wire format (snake_case) and domain tokens. */
export class ApiTokenMapper {
  static token(dto: ApiTokenDto): ApiToken {
    return {
      id: dto.id,
      name: dto.name,
      prefix: dto.prefix,
      scopes: dto.scopes,
      createdAt: dto.created_at,
      expiresAt: dto.expires_at ?? null,
      lastUsedAt: dto.last_used_at ?? null,
      status: dto.status,
    };
  }

  static created(dto: ApiTokenCreatedDto): CreatedApiToken {
    return { token: ApiTokenMapper.token(dto), secret: dto.token };
  }

  static request(draft: ApiTokenDraft): ApiTokenCreateDto {
    return { name: draft.name.trim(), scopes: [draft.scope], expires_in_days: draft.expiresInDays };
  }
}
