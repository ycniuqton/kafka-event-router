import { Format } from '../../../components/ui';
import { LagHealth, RouteEndpointText, RouteStatus, Tone, TransformMode, TransformModeMeta, type Route } from '../../../domain';

/** Time ranges of the overview (spec §10.1) and their length in hours. */
export const TimeRange = { M15: '15m', H1: '1h', H24: '24h', D7: '7d' } as const;
export type TimeRange = (typeof TimeRange)[keyof typeof TimeRange];

export const TimeRangeHours: Record<TimeRange, number> = {
  [TimeRange.M15]: 0.25,
  [TimeRange.H1]: 1,
  [TimeRange.H24]: 24,
  [TimeRange.D7]: 168,
};

export const TimeRangeOptions: readonly TimeRange[] = [TimeRange.M15, TimeRange.H1, TimeRange.H24, TimeRange.D7];

/** Event counts on each arrow of the pipeline flow for a range. */
export type FlowCounts = { read: number; passed: number; sent: number; deadLetters: number };

/** Latency of the route in ms, by transform kind (JS steps run in a sandbox and are slower). */
export const RouteLatency = {
  js: { p50: 18, p95: 64 },
  plain: { p50: 6, p95: 21 },
} as const;

/** Header and overview numbers of one route (design `detailVals`). */
export class RouteDetailModel {
  static readonly secondsPerHour = 3600;
  static readonly idleEventsPerSec = 1200;
  static readonly idleShare = 0.02;
  static readonly percent = 100;
  static readonly errorsPerMinOnError = 2.4;
  static readonly topicJoiner = ' + ';

  readonly route: Route;

  constructor(route: Route) {
    this.route = route;
  }

  get canPause(): boolean {
    return this.route.status !== RouteStatus.Draft && this.route.status !== RouteStatus.Archived;
  }

  get sourceText(): string {
    return `${RouteEndpointText.sourceConnection(this.route)} / ${this.route.source.topics.join(RouteDetailModel.topicJoiner)}`;
  }

  get destinationText(): string {
    return RouteEndpointText.destination(this.route);
  }

  get transformLabel(): string {
    return TransformModeMeta[this.route.transformMode].long;
  }

  get eventsIn(): number {
    const share = this.route.metrics.matchPct / RouteDetailModel.percent || 1;
    return Math.round(this.route.metrics.eventsPerSec / share);
  }

  get eventsOut(): number {
    return this.route.metrics.eventsPerSec;
  }

  get latency(): { p50: number; p95: number } {
    return this.route.transformMode === TransformMode.Js ? RouteLatency.js : RouteLatency.plain;
  }

  get errorsPerMin(): number {
    return this.route.status === RouteStatus.Error ? RouteDetailModel.errorsPerMinOnError : 0;
  }

  get errorTone(): Tone {
    return this.errorsPerMin > 0 ? Tone.Err : Tone.Text;
  }

  get lagTone(): Tone {
    const tone = LagHealth.routeTone(this.route.metrics.lag);
    return tone === Tone.TextSecondary ? Tone.Text : tone;
  }

  get lagText(): string {
    return Format.number(this.route.metrics.lag);
  }

  flow(range: TimeRange, deadLetters: number): FlowCounts {
    const hours = TimeRangeHours[range];
    const perHour = RouteDetailModel.secondsPerHour * hours;
    const idle = Math.round(RouteDetailModel.idleEventsPerSec * perHour * RouteDetailModel.idleShare);
    const read = Math.round(this.eventsIn * perHour) || idle;
    const passed = Math.round((read * this.route.metrics.matchPct) / RouteDetailModel.percent);
    return { read, passed, sent: Math.max(0, passed - deadLetters), deadLetters };
  }
}
