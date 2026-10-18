import type { Services } from '../../contracts';
import type { HttpContext } from './HttpContext';

export type ServiceKey = keyof Services;

/** Service keys of `Services`, for modules' `key`. */
export const ServiceKeys = {
  Routes: 'routes',
  Connections: 'connections',
  DeadLetters: 'deadLetters',
  Activity: 'activity',
  Audit: 'audit',
  Library: 'library',
  Samples: 'samples',
  Preview: 'preview',
  Alerts: 'alerts',
  Metrics: 'metrics',
  Push: 'push',
  LiveTail: 'liveTail',
  Onboarding: 'onboarding',
  Members: 'members',
  Assist: 'assist',
  TestCases: 'testCases',
  Runtime: 'runtime',
  Sandbox: 'sandbox',
  ApiTokens: 'apiTokens',
  Clock: 'clock',
} as const satisfies Record<string, ServiceKey>;

/**
 * Base for a feature's REST service contribution. A feature adds
 * `web/src/api/http/<domain>/module.ts` default-exporting an instance (auto-discovered),
 * or calls `HttpServicesFactory.provide(module)`.
 */
export abstract class HttpServiceModule<K extends ServiceKey = ServiceKey> {
  abstract readonly key: K;
  abstract create(context: HttpContext, fallback: () => Services): Services[K];
}
