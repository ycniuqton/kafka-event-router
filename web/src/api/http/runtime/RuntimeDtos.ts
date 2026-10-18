import type { HaltReason, ObservedState, RunnerState } from '../../../domain';

/** Endpoints of route-runtime (relative to the API root). */
export class RuntimeEndpoint {
  static readonly workers = '/workers';
  static readonly routes = '/routes';
  static readonly runtimeAction = 'runtime';

  static route(id: string): string {
    return `${RuntimeEndpoint.routes}/${encodeURIComponent(id)}/${RuntimeEndpoint.runtimeAction}`;
  }
}

export type ReplicaDto = {
  workerId: string;
  state: RunnerState;
  versionNo: number | null;
  lag: number;
  reportedAt: string | null;
  errorMessage?: string;
};

export type RouteRuntimeDto = {
  observedState: ObservedState;
  runningVersionNo: number | null;
  haltedAt?: string;
  haltReason?: HaltReason;
  errorMessage?: string;
  lag: number;
  replicas: ReplicaDto[];
};

export type WorkerDto = {
  id: string;
  host: string;
  zone: string;
  version: string;
  capacity: number;
  capacityUsed: number;
  draining: boolean;
  alive: boolean;
  lastHeartbeatAt: string;
  routes: number;
};
