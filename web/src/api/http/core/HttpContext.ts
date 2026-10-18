import { ApiConfig } from '../../config';
import { ApiErrorBus } from './ApiErrorBus';
import { CsrfTokenStore } from './CsrfTokenStore';
import { EtagStore } from './EtagStore';
import { HttpClient, type FetchFn } from './HttpClient';

/** API roots: `/api/v1` for resources, the site root for the browser-facing `/auth/*` endpoints. */
export class ApiRoot {
  static readonly defaultAuthBaseUrl = '';

  static api(): string {
    return ApiConfig.baseUrl();
  }

  static auth(): string {
    return import.meta.env.VITE_AUTH_BASE_URL ?? ApiRoot.defaultAuthBaseUrl;
  }
}

export type HttpContextOptions = { apiBaseUrl?: string; authBaseUrl?: string; fetch?: FetchFn };

/** Everything an HTTP service needs: the two clients and the shared CSRF/ETag/error state. */
export class HttpContext {
  readonly errors = new ApiErrorBus();
  readonly csrf = new CsrfTokenStore();
  readonly etags = new EtagStore();
  readonly api: HttpClient;
  readonly auth: HttpClient;

  constructor(options: HttpContextOptions = {}) {
    const shared = { csrf: this.csrf, errors: this.errors, fetch: options.fetch };
    this.api = new HttpClient({ ...shared, baseUrl: options.apiBaseUrl ?? ApiRoot.api() });
    this.auth = new HttpClient({ ...shared, baseUrl: options.authBaseUrl ?? ApiRoot.auth() });
  }
}

/** The process-wide HTTP context (created on first use). */
export class HttpRuntime {
  private static instance: HttpContext | null = null;

  static context(): HttpContext {
    HttpRuntime.instance ??= new HttpContext();
    return HttpRuntime.instance;
  }
}
