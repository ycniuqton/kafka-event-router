import { Clock, type Activity, type AuditEntry, type Connection, type DeadLetter, type Route, type RouteVersion } from '../../domain';
import { ACTIVITY, AUDIT, CONNECTIONS, DEAD_LETTERS, MockTime, ROUTES, VERSIONS } from '../../mocks';
import { MockLatency } from '../config';

/** Clock anchored at the mock reference instant, advancing in real time since page load. */
export class MockClock extends Clock {
  private readonly bootedAt = Date.now();

  now(): Date {
    return new Date(MockTime.reference + (Date.now() - this.bootedAt));
  }
}

/** Mutable in-memory copy of the seed data, shared by all mock services. */
export class MockDatabase {
  routes: Route[] = structuredClone(ROUTES);
  connections: Connection[] = structuredClone(CONNECTIONS);
  deadLetters: DeadLetter[] = structuredClone(DEAD_LETTERS);
  activity: Activity[] = structuredClone(ACTIVITY);
  audit: AuditEntry[] = structuredClone(AUDIT);
  versions: Record<string, RouteVersion[]> = {};

  /** Version history of a route; routes without seeded history start from the design's list. */
  historyOf(routeId: string): RouteVersion[] {
    this.versions[routeId] ??= VERSIONS.map((v) => ({ ...structuredClone(v), routeId }));
    return this.versions[routeId];
  }
}

/** Shared state and helpers of the mock backend: database, clock and simulated latency. */
export class MockBackend {
  readonly db = new MockDatabase();
  readonly clock = new MockClock();

  /** Resolves with a deep copy after the simulated request latency. */
  reply<T>(value: T, delayMs: number = MockLatency.requestMs): Promise<T> {
    return new Promise((resolve) => setTimeout(() => resolve(structuredClone(value)), delayMs));
  }

  fail(message: string): Promise<never> {
    return new Promise((_, reject) => setTimeout(() => reject(new Error(message)), MockLatency.requestMs));
  }

  nowIso(): string {
    return this.clock.now().toISOString();
  }
}
