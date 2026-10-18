/** Prefix of everything the suite creates on shared infrastructure (topics, routes, connections, users). */
export const AccPrefix = 'acc-' as const;

/** Unique, readable names for one test: acc-<area>-<stamp><rand>-<suffix>. */
export class AccNames {
  readonly base: string;

  constructor(area: string) {
    const rand = Math.random().toString(36).slice(2, 6);
    this.base = `${AccPrefix}${area}-${Date.now().toString(36)}${rand}`;
  }

  of(suffix: string): string {
    return `${this.base}-${suffix}`;
  }

  get route(): string {
    return this.of('route');
  }

  get connection(): string {
    return this.of('conn');
  }

  get inTopic(): string {
    return this.of('in');
  }

  get outTopic(): string {
    return this.of('out');
  }

  /** Default DLQ topic of a route (route-runtime FR-16). */
  dlqTopic(route: string = this.route): string {
    return `router.dlq.${route}`;
  }

  /** Default consumer group of a route. */
  group(route: string = this.route): string {
    return `router.${route}`;
  }

  email(role: string): string {
    return `${this.base}-${role}@local.dev`;
  }
}
