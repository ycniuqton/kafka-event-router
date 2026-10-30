import type { RetryPolicySettings } from './retryPolicy';

/** What happens to an event a step fails on (route document `errorPolicy.onError`). */
export const ErrorAction = { Skip: 'skip', DeadLetter: 'dlq', Pause: 'pause' } as const;
export type ErrorAction = (typeof ErrorAction)[keyof typeof ErrorAction];

/** Error handling of a route; empty DLQ connection means the source connection. */
export type RouteErrorPolicy = {
  onError: ErrorAction;
  dlqConnectionId: string;
  dlqTopic: string;
  maxRetries: number | null;
  /** Auto-retry of transient failures; undefined keeps the document's value, null removes it. */
  retry?: RetryPolicySettings | null;
};

/** Per-route processing limits; null keeps the engine default. */
export type RouteLimits = {
  maxInFlight: number | null;
  minInFlight: number | null;
  jsTimeoutMs: number | null;
  jsMemoryMb: number | null;
  jsMinInstances: number | null;
  jsMaxInstances: number | null;
  maxOutputsPerEvent: number | null;
  maxOutputBytes: number | null;
};

/** How the in-flight limit is sized (route document `limits.inFlightMode`); absent means auto. */
export const InFlightMode = { Auto: 'auto', Fixed: 'fixed' } as const;
export type InFlightMode = (typeof InFlightMode)[keyof typeof InFlightMode];

/** Delivery guarantee of a route (route document `delivery.mode`). */
export const DeliveryMode = { AtLeastOnce: 'at_least_once', ExactlyOnce: 'exactly_once' } as const;
export type DeliveryMode = (typeof DeliveryMode)[keyof typeof DeliveryMode];

/** Route detail Settings tab: metadata saved immediately, error policy, delivery and limits saved into the draft. */
export type RouteSettings = {
  name: string;
  description: string;
  tags: string[];
  replicas: number;
  errorPolicy: RouteErrorPolicy;
  deliveryMode: DeliveryMode;
  inFlightMode: InFlightMode;
  limits: RouteLimits;
};

export type RouteSettingsPatch = Partial<RouteSettings>;

/** Labels, bounds and parsing rules of the Settings form. */
export class RouteSettingsRules {
  static readonly minReplicas = 1;
  static readonly maxReplicas = 64;
  static readonly maxTagLength = 64;
  static readonly tagSeparator = ',';

  static readonly errorActionLabels: Record<ErrorAction, string> = {
    [ErrorAction.Skip]: 'Skip and log',
    [ErrorAction.DeadLetter]: 'Dead-letter topic',
    [ErrorAction.Pause]: 'Pause route',
  };

  static readonly deliveryModeLabels: Record<DeliveryMode, string> = {
    [DeliveryMode.AtLeastOnce]: 'At least once',
    [DeliveryMode.ExactlyOnce]: 'Exactly once',
  };

  static readonly deliveryModeHints: Record<DeliveryMode, string> = {
    [DeliveryMode.AtLeastOnce]: 'Every event is delivered; after a restart or retry some events can arrive twice.',
    [DeliveryMode.ExactlyOnce]:
      'Outputs, dead letters and source offsets are written in one Kafka transaction. Consumers of the destination must read with isolation.level=read_committed, otherwise they also see aborted writes.',
  };

  static readonly exactlyOnceSameConnection = 'Exactly once needs every destination and the dead-letter topic on the source connection.';

  /** Connections other than the source that an exactly-once route would write to (must be empty). */
  static foreignConnections(sourceId: string, destinationIds: string[], dlqConnectionId: string): string[] {
    return [...new Set([...destinationIds, dlqConnectionId].filter((id) => id && id !== sourceId))];
  }

  static readonly limitLabels: Record<keyof RouteLimits, string> = {
    maxInFlight: 'Max in-flight events',
    minInFlight: 'Min in-flight events',
    jsTimeoutMs: 'JS timeout (ms)',
    jsMemoryMb: 'JS memory (MB)',
    jsMinInstances: 'Min JS instances',
    jsMaxInstances: 'Max JS instances',
    maxOutputsPerEvent: 'Max outputs per event',
    maxOutputBytes: 'Max output size (bytes)',
  };

  /** Field hints; in auto mode max in-flight is the cap of the adaptive limit. */
  static limitHint(key: keyof RouteLimits, mode: InFlightMode): string {
    if (key === 'maxInFlight') return mode === InFlightMode.Auto ? `Cap of the auto limit · empty: ${RouteSettingsRules.autoMaxInFlight}` : `Empty: ${RouteSettingsRules.fixedInFlight}`;
    if (key === 'minInFlight') return mode === InFlightMode.Auto ? `Floor of the auto limit · empty: ${RouteSettingsRules.autoMinInFlight}` : 'Not used in fixed mode';
    if (key === 'jsMaxInstances') return 'Per JS step · empty: worker cores';
    if (key === 'jsMinInstances') return 'Per JS step · empty: 1';
    return 'Empty: default';
  }

  static readonly autoMaxInFlight = 10_000;
  static readonly autoMinInFlight = 64;
  static readonly fixedInFlight = 1_000;

  static readonly inFlightModeLabels: Record<InFlightMode, string> = {
    [InFlightMode.Auto]: 'Auto-tune',
    [InFlightMode.Fixed]: 'Fixed',
  };

  static readonly emptyLimits: RouteLimits = {
    maxInFlight: null,
    minInFlight: null,
    jsTimeoutMs: null,
    jsMemoryMb: null,
    jsMinInstances: null,
    jsMaxInstances: null,
    maxOutputsPerEvent: null,
    maxOutputBytes: null,
  };

  static readonly defaultErrorPolicy: RouteErrorPolicy = { onError: ErrorAction.DeadLetter, dlqConnectionId: '', dlqTopic: '', maxRetries: null };

  /** "pii, payments" → ["pii", "payments"] (trimmed, unique, empties dropped). */
  static parseTags(text: string): string[] {
    return [...new Set(text.split(RouteSettingsRules.tagSeparator).map((t) => t.trim()).filter(Boolean))];
  }

  /** Empty input → null (engine default), else a positive integer or NaN when invalid. */
  static parseLimit(text: string): number | null {
    const trimmed = text.trim();
    return trimmed ? Number(trimmed) : null;
  }

  static validLimit(value: number | null): boolean {
    return value === null || (Number.isInteger(value) && value > 0);
  }

  static validReplicas(value: number): boolean {
    return Number.isInteger(value) && value >= RouteSettingsRules.minReplicas && value <= RouteSettingsRules.maxReplicas;
  }
}
