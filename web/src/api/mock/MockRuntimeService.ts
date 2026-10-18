import { RuntimeDefaults, RuntimeEstimator, type RouteRuntime, type WorkerInfo } from '../../domain';
import { RuntimeService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Mock fleet: two workers in two zones sharing the running routes. */
const MockFleet = { Zones: ['zone-a', 'zone-b'], Capacity: 100, Version: 'dev', HostSuffix: '.local' } as const;

/** Mock `services.runtime`: derives replica status from the seeded routes. */
export class MockRuntimeService extends RuntimeService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  get(routeId: string): Promise<RouteRuntime> {
    const route = this.backend.db.routes.find((r) => r.id === routeId);
    if (!route) return this.backend.fail(`Route ${routeId} not found`);
    return this.backend.reply(RuntimeEstimator.fromRoute(route, this.backend.nowIso()));
  }

  workers(): Promise<WorkerInfo[]> {
    const now = this.backend.nowIso();
    const live = this.backend.db.routes.map((r) => RuntimeEstimator.fromRoute(r, now)).filter((rt) => rt.replicas.length > 0);
    const workers = MockFleet.Zones.map((zone, i) => {
      const id = `${RuntimeDefaults.WorkerPrefix}${i + 1}`;
      return {
        id,
        host: `${id}${MockFleet.HostSuffix}`,
        zone,
        version: MockFleet.Version,
        capacity: MockFleet.Capacity,
        capacityUsed: live.length,
        draining: false,
        alive: true,
        lastHeartbeatAt: now,
        routes: live.length,
      };
    });
    return this.backend.reply(workers);
  }
}
