import { ApiTokenStatus, type ApiToken, type ApiTokenDraft, type CreatedApiToken } from '../../domain';
import { ApiTokensService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Shape and copy of mock tokens (mirrors the server: `ert_` + 43 base64url chars, 12-char prefix). */
class MockTokenFormat {
  static readonly prefix = 'ert_';
  static readonly secretChars = 43;
  static readonly displayChars = 12;
  static readonly alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
  static readonly dayMs = 86_400_000;
  static readonly notFound = 'This API token does not exist or was already revoked.';
  static readonly duplicate = 'You already have a token with this name. Use a different name.';

  static secret(): string {
    const bytes = crypto.getRandomValues(new Uint8Array(MockTokenFormat.secretChars));
    return MockTokenFormat.prefix + Array.from(bytes, (b) => MockTokenFormat.alphabet[b % MockTokenFormat.alphabet.length]).join('');
  }
}

/** Mock `services.apiTokens`: in-memory personal tokens of the signed-in mock user. */
export class MockApiTokensService extends ApiTokensService {
  private readonly backend: MockBackend;
  private tokens: ApiToken[] = [];

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(): Promise<ApiToken[]> {
    return this.backend.reply(this.tokens);
  }

  create(draft: ApiTokenDraft): Promise<CreatedApiToken> {
    const name = draft.name.trim();
    if (this.tokens.some((t) => t.name === name)) return this.backend.fail(MockTokenFormat.duplicate);
    const secret = MockTokenFormat.secret();
    const now = this.backend.clock.now();
    const token: ApiToken = {
      id: crypto.randomUUID(),
      name,
      prefix: secret.slice(0, MockTokenFormat.displayChars),
      scopes: [draft.scope],
      createdAt: now.toISOString(),
      expiresAt: new Date(now.getTime() + draft.expiresInDays * MockTokenFormat.dayMs).toISOString(),
      lastUsedAt: null,
      status: ApiTokenStatus.Active,
    };
    this.tokens = [token, ...this.tokens];
    return this.backend.reply({ token, secret });
  }

  revoke(id: string): Promise<void> {
    if (!this.tokens.some((t) => t.id === id)) return this.backend.fail(MockTokenFormat.notFound);
    this.tokens = this.tokens.filter((t) => t.id !== id);
    return this.backend.reply(undefined);
  }
}
