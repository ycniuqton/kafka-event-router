import { ApiConfig, DataSource } from '../../api';
import { HttpRuntime, HttpSessionService, type HttpContext, type SessionService } from '../../api/http/core';
import { MockSessionService } from './MockSessionService';

type SessionFactory = () => SessionService;

/** Manager for the session service: picks the implementation registered for the configured data source. */
export class SessionServices {
  private static readonly factories = new Map<string, SessionFactory>([
    [DataSource.Mock, () => new MockSessionService()],
    [DataSource.Http, () => new HttpSessionService(HttpRuntime.context())],
  ]);
  private static active: SessionService | null = null;

  static register(source: DataSource, factory: SessionFactory): void {
    SessionServices.factories.set(source, factory);
  }

  static current(): SessionService {
    SessionServices.active ??= SessionServices.resolve(ApiConfig.dataSource());
    return SessionServices.active;
  }

  /** The shared HTTP context whose failures drive global handling (idle with the mock data source). */
  static httpContext(): HttpContext {
    return HttpRuntime.context();
  }

  private static resolve(source: string): SessionService {
    const factory = SessionServices.factories.get(source);
    if (!factory) throw new Error(`No session service registered for data source "${source}"`);
    return factory();
  }
}
