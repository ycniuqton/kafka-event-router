import type { Services } from '../../contracts';
import { MockServicesFactory } from '../../mock';
import { HttpRuntime, type HttpContext } from './HttpContext';
import type { HttpServiceModule, ServiceKey } from './HttpServiceModule';
import { HttpClockModule } from './HttpClockModule';
import { HttpMembersModule } from './session/HttpMembersModule';

/** Feature modules found by convention: `web/src/api/http/<domain>/module.ts` (default export). */
const discovered = import.meta.glob<HttpServiceModule>('../*/module.ts', { eager: true, import: 'default' });

/**
 * The Manager for `DataSource.Http`: one module per service key (later registrations win).
 * Keys without a REST module yet keep their mock service, with a console warning.
 */
export class HttpServicesFactory {
  private static readonly modules = new Map<ServiceKey, HttpServiceModule>();
  private static readonly warning = '[api/http] no REST module yet, using mock for:';

  static {
    [new HttpClockModule(), new HttpMembersModule(), ...Object.values(discovered)].forEach((m) => HttpServicesFactory.provide(m));
  }

  static provide(module: HttpServiceModule): void {
    HttpServicesFactory.modules.set(module.key, module);
  }

  static provided(): ServiceKey[] {
    return [...HttpServicesFactory.modules.keys()];
  }

  static create(context: HttpContext = HttpRuntime.context()): Services {
    const fallback = MockServicesFactory.create();
    const services: Services = { ...fallback };
    const set = services as Record<ServiceKey, Services[ServiceKey]>;
    HttpServicesFactory.modules.forEach((module, key) => {
      set[key] = module.create(context, () => fallback);
    });
    const missing = Object.keys(fallback).filter((key) => !HttpServicesFactory.modules.has(key as ServiceKey));
    if (missing.length > 0) console.warn(HttpServicesFactory.warning, missing.join(', '));
    return services;
  }
}
