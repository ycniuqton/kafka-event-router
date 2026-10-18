import { RouteStatus, type Route } from './route';
import { Tone } from './tone';

/** State of one route runner on one worker (server `RunnerState`). */
export const RunnerState = {
  Starting: 'starting',
  Running: 'running',
  Draining: 'draining',
  Paused: 'paused',
  Error: 'error',
  Stopped: 'stopped',
} as const;
export type RunnerState = (typeof RunnerState)[keyof typeof RunnerState];

/** Aggregated state of a route across its replicas (server `route_observed_state`). */
export const ObservedState = { Running: 'running', Paused: 'paused', Error: 'error' } as const;
export type ObservedState = (typeof ObservedState)[keyof typeof ObservedState];

/** Why the runtime halted a route without a restart loop (resume or a new deploy lifts it). */
export const HaltReason = {
  ErrorPolicyPause: 'error_policy_pause',
  DestinationPermanent: 'destination_permanent',
  SourceFatal: 'source_fatal',
  AutoRollbackFailed: 'auto_rollback_failed',
  DlqTopicUnavailable: 'dlq_topic_unavailable',
  DeliveryModeForbidden: 'delivery_mode_forbidden',
} as const;
export type HaltReason = (typeof HaltReason)[keyof typeof HaltReason];

/** One replica of a route as reported by its worker. */
export type ReplicaStatus = {
  workerId: string;
  state: RunnerState;
  versionNo: number | null;
  lag: number;
  reportedAt: string | null;
  errorMessage: string | null;
};

/** Runtime view of a route (`GET /routes/{id}/runtime`). */
export type RouteRuntime = {
  observedState: ObservedState;
  runningVersionNo: number | null;
  haltedAt: string | null;
  haltReason: HaltReason | null;
  errorMessage: string | null;
  lag: number;
  replicas: ReplicaStatus[];
};

/** One data-plane worker (`GET /workers`, workspace admins). */
export type WorkerInfo = {
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

/** Labels for halt reasons, as shown next to an error badge. */
export const HaltReasonLabel: Record<HaltReason, string> = {
  [HaltReason.ErrorPolicyPause]: 'Paused by error policy',
  [HaltReason.DestinationPermanent]: 'Destination unavailable',
  [HaltReason.SourceFatal]: 'Source unreadable',
  [HaltReason.AutoRollbackFailed]: 'Rollout failed, no version to roll back to',
  [HaltReason.DlqTopicUnavailable]: 'Dead-letter topic cannot be created',
  [HaltReason.DeliveryModeForbidden]: 'Delivery mode not allowed',
};

/** Label and tone per runner state. */
export const RunnerStateMeta: Record<RunnerState, { label: string; tone: Tone }> = {
  [RunnerState.Starting]: { label: 'Starting', tone: Tone.Info },
  [RunnerState.Running]: { label: 'Running', tone: Tone.Ok },
  [RunnerState.Draining]: { label: 'Draining', tone: Tone.Warn },
  [RunnerState.Paused]: { label: 'Paused', tone: Tone.Off },
  [RunnerState.Error]: { label: 'Error', tone: Tone.Err },
  [RunnerState.Stopped]: { label: 'Stopped', tone: Tone.Off },
};

/** Label and tone per aggregated route state. */
export const ObservedStateMeta: Record<ObservedState, { label: string; tone: Tone }> = {
  [ObservedState.Running]: { label: 'Running', tone: Tone.Ok },
  [ObservedState.Paused]: { label: 'Stopped', tone: Tone.Off },
  [ObservedState.Error]: { label: 'Error', tone: Tone.Err },
};

/** Worker liveness as shown in the workers list. */
export const WorkerHealthMeta = {
  alive: { label: 'Alive', tone: Tone.Ok },
  draining: { label: 'Draining', tone: Tone.Warn },
  dead: { label: 'No heartbeat', tone: Tone.Err },
} as const satisfies Record<string, { label: string; tone: Tone }>;

/** Polling cadence of runtime views (the worker reports status every few seconds). */
export const RuntimeRefresh = { Ms: 10_000 } as const;

/** Defaults of estimated runtime views: server default `replicas = 2`, worker naming of the compose stack. */
export const RuntimeDefaults = { Replicas: 2, PausedLag: 0, WorkerPrefix: 'worker-' } as const;

/** Derives a plausible runtime view from a listed route (mock data source only; the http source never falls back to it). */
export class RuntimeEstimator {
  static fromRoute(route: Route, reportedAt: string): RouteRuntime {
    const observed = RuntimeEstimator.observed(route.status);
    const state = RuntimeEstimator.runnerState(observed);
    const live = observed !== ObservedState.Paused;
    const replicas: ReplicaStatus[] = live
      ? Array.from({ length: RuntimeDefaults.Replicas }, (_, i) => ({
          workerId: `${RuntimeDefaults.WorkerPrefix}${i + 1}`,
          state,
          versionNo: route.version,
          lag: Math.round(route.metrics.lag / RuntimeDefaults.Replicas),
          reportedAt,
          errorMessage: null,
        }))
      : [];
    return {
      observedState: observed,
      runningVersionNo: live ? route.version : null,
      haltedAt: null,
      haltReason: null,
      errorMessage: null,
      lag: live ? route.metrics.lag : RuntimeDefaults.PausedLag,
      replicas,
    };
  }

  private static observed(status: RouteStatus): ObservedState {
    if (status === RouteStatus.Running) return ObservedState.Running;
    if (status === RouteStatus.Error) return ObservedState.Error;
    return ObservedState.Paused;
  }

  private static runnerState(observed: ObservedState): RunnerState {
    if (observed === ObservedState.Error) return RunnerState.Error;
    if (observed === ObservedState.Running) return RunnerState.Running;
    return RunnerState.Paused;
  }
}
