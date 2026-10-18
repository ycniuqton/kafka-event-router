import { request as playwrightRequest, type APIRequestContext, type BrowserContext } from '@playwright/test';
import { AuthPaths, LoginManager } from './Login';

/** CSRF double-submit names the API expects on cookie-authenticated writes. */
export const Csrf = { header: 'X-CSRF-Token', cookie: 'er_csrf' } as const;

/** HTTP methods used by the suite. */
export enum Http {
  Get = 'GET',
  Post = 'POST',
  Put = 'PUT',
  Patch = 'PATCH',
  Delete = 'DELETE',
}

/** One API answer: status, parsed body (JSON when possible, else text), ETag and raw text. */
export class ApiResponse<T = any> {
  constructor(
    readonly status: number,
    readonly body: T,
    readonly text: string,
    readonly headers: Record<string, string>,
  ) {}

  get etag(): string {
    return this.headers['etag'] ?? '';
  }

  get ok(): boolean {
    return this.status >= 200 && this.status < 300;
  }

  /** Readable form for assertion messages. */
  describe(): string {
    return `${this.status} ${this.text.slice(0, 800)}`;
  }
}

/** The signed-in principal from GET /auth/me. */
export type Me = { id: string; email: string; teams: { id: string; name: string; role: string }[]; access: { workspaceAdmin: boolean; teams: Record<string, { role: string; permissions: string[] }> } };

/** A JSON API session bound to one browser context's cookies, or to an API token (CSRF + If-Match handled here). */
export class AccApi {
  private meCache: Me | null = null;

  constructor(
    readonly context: BrowserContext | null,
    private readonly bearer: string | null = null,
    private readonly tokenRequest: APIRequestContext | null = null,
  ) {}

  get request(): APIRequestContext {
    return this.tokenRequest ?? this.context!.request;
  }

  /** Signs the context in with the target's login method (email = impersonated user, local only). */
  static async signIn(context: BrowserContext, email?: string): Promise<AccApi> {
    await LoginManager.forProfile().signIn(context, email);
    return new AccApi(context);
  }

  /** A cookie-less session authenticated only by an API token (dispose it after use). */
  static async withToken(baseURL: string, token: string): Promise<AccApi> {
    return new AccApi(null, token, await playwrightRequest.newContext({ baseURL }));
  }

  async dispose(): Promise<void> {
    await this.tokenRequest?.dispose();
  }

  async call<T = any>(method: Http, path: string, body?: unknown, ifMatch?: string): Promise<ApiResponse<T>> {
    const headers = await this.headers(ifMatch);
    const res = await this.request.fetch(path, { method, headers, data: body === undefined ? undefined : body, failOnStatusCode: false, maxRedirects: 0 });
    const text = await res.text();
    return new ApiResponse<T>(res.status(), AccApi.parse(text) as T, text, res.headers());
  }

  get<T = any>(path: string): Promise<ApiResponse<T>> {
    return this.call<T>(Http.Get, path);
  }

  post<T = any>(path: string, body: unknown = {}, ifMatch?: string): Promise<ApiResponse<T>> {
    return this.call<T>(Http.Post, path, body, ifMatch);
  }

  patch<T = any>(path: string, body: unknown, ifMatch?: string): Promise<ApiResponse<T>> {
    return this.call<T>(Http.Patch, path, body, ifMatch);
  }

  put<T = any>(path: string, body: unknown, ifMatch?: string): Promise<ApiResponse<T>> {
    return this.call<T>(Http.Put, path, body, ifMatch);
  }

  delete<T = any>(path: string, ifMatch?: string): Promise<ApiResponse<T>> {
    return this.call<T>(Http.Delete, path, undefined, ifMatch);
  }

  /** Writes that need the current ETag of `path` (GET first). */
  async withEtag<T = any>(method: Http, path: string, body?: unknown, etagOf: string = path): Promise<ApiResponse<T>> {
    const current = await this.get(etagOf);
    return this.call<T>(method, path, body, current.etag);
  }

  async me(): Promise<Me> {
    if (!this.meCache) this.meCache = (await this.get<Me>(AuthPaths.me)).body;
    return this.meCache;
  }

  /** The caller's first team (the default team for dev login). */
  async teamId(): Promise<string> {
    return (await this.me()).teams[0].id;
  }

  private async headers(ifMatch?: string): Promise<Record<string, string>> {
    const headers: Record<string, string> = {};
    if (this.bearer) headers['Authorization'] = `Bearer ${this.bearer}`;
    else headers[Csrf.header] = await this.csrf();
    if (ifMatch) headers['If-Match'] = ifMatch;
    return headers;
  }

  private async csrf(): Promise<string> {
    if (!this.context) return '';
    const cookies = await this.context.cookies();
    return cookies.find((c) => c.name === Csrf.cookie)?.value ?? '';
  }

  private static parse(text: string): unknown {
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  }
}
