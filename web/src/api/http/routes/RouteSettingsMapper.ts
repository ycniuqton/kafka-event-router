import type { RouteConfig } from '../../generated';
import { DeliveryMode, ErrorAction, InFlightMode, RetryPolicyRules, RouteSettingsRules, type RetryPolicySettings, type RouteErrorPolicy, type RouteLimits, type RouteSettings, type RouteSettingsPatch } from '../../../domain';
import type { RouteDetailDto } from './RouteDtos';

/** PATCH /routes/{id} body of the Settings tab. */
export type SettingsPatchBody = {
  name?: string;
  description?: string;
  tags?: string[];
  replicas?: number;
  draft?: RouteConfig;
};

/** Maps a route detail to the Settings tab model and a settings patch back to a PATCH body. */
export class RouteSettingsMapper {
  static fromDetail(dto: RouteDetailDto): RouteSettings {
    const doc = RouteSettingsMapper.document(dto);
    return {
      name: dto.name,
      description: dto.description ?? '',
      tags: dto.tags ?? [],
      replicas: dto.replicas,
      errorPolicy: RouteSettingsMapper.errorPolicy(doc),
      deliveryMode: RouteSettingsMapper.deliveryMode(doc),
      inFlightMode: doc?.limits?.inFlightMode ?? InFlightMode.Auto,
      limits: {
        ...RouteSettingsRules.emptyLimits,
        ...RouteSettingsMapper.limits(doc),
      },
    };
  }

  /** Metadata goes top-level; error policy, delivery mode and limits rewrite the draft (or the active document). */
  static toBody(dto: RouteDetailDto, patch: RouteSettingsPatch): SettingsPatchBody {
    const body: SettingsPatchBody = {
      name: patch.name,
      description: patch.description,
      tags: patch.tags,
      replicas: patch.replicas,
    };
    const doc = RouteSettingsMapper.document(dto);
    if (doc && (patch.errorPolicy || patch.limits || patch.deliveryMode || patch.inFlightMode)) body.draft = RouteSettingsMapper.applyToDocument(doc, patch);
    return body;
  }

  static document(dto: RouteDetailDto): RouteConfig | null {
    return dto.draft ?? dto.active ?? null;
  }

  private static applyToDocument(doc: RouteConfig, patch: RouteSettingsPatch): RouteConfig {
    const next: RouteConfig = { ...doc };
    if (patch.errorPolicy) next.errorPolicy = RouteSettingsMapper.errorPolicyDoc(patch.errorPolicy, doc.errorPolicy);
    if (patch.limits || patch.inFlightMode) next.limits = RouteSettingsMapper.limitsDoc(patch.limits ?? RouteSettingsMapper.limits(doc), patch.inFlightMode ?? doc.limits?.inFlightMode);
    if (patch.deliveryMode && patch.deliveryMode !== RouteSettingsMapper.deliveryMode(doc)) next.delivery = { mode: patch.deliveryMode };
    if (next.limits && !Object.keys(next.limits).length) delete next.limits;
    return next;
  }

  /** Absent `delivery` means at least once. */
  static deliveryMode(doc: RouteConfig | null): DeliveryMode {
    return doc?.delivery?.mode ?? DeliveryMode.AtLeastOnce;
  }

  private static errorPolicy(doc: RouteConfig | null): RouteErrorPolicy {
    const p = doc?.errorPolicy;
    if (!p) return RouteSettingsRules.defaultErrorPolicy;
    return {
      onError: p.onError ?? ErrorAction.DeadLetter,
      dlqConnectionId: p.dlqConnectionId ?? '',
      dlqTopic: p.dlqTopic ?? '',
      maxRetries: p.maxRetries ?? null,
      retry: p.retry ? RouteSettingsMapper.retry(p.retry) : null,
    };
  }

  /** Document retry policy with the schema defaults filled in. */
  private static retry(r: NonNullable<NonNullable<RouteConfig['errorPolicy']>['retry']>): RetryPolicySettings {
    const d = RetryPolicyRules.defaults;
    return { enabled: r.enabled, maxAttempts: r.maxAttempts ?? d.maxAttempts, baseDelayMs: r.baseDelayMs ?? d.baseDelayMs, maxDelayMs: r.maxDelayMs ?? d.maxDelayMs };
  }

  private static errorPolicyDoc(policy: RouteErrorPolicy, base: RouteConfig['errorPolicy']): NonNullable<RouteConfig['errorPolicy']> {
    const out: NonNullable<RouteConfig['errorPolicy']> = {
      ...base,
      onError: policy.onError,
    };
    RouteSettingsMapper.setOptional(out, 'dlqConnectionId', policy.dlqConnectionId || undefined);
    RouteSettingsMapper.setOptional(out, 'dlqTopic', policy.dlqTopic || undefined);
    RouteSettingsMapper.setOptional(out, 'maxRetries', policy.maxRetries ?? undefined);
    if (policy.retry !== undefined) RouteSettingsMapper.setOptional(out, 'retry', policy.retry ?? undefined);
    return out;
  }

  /** The numeric limits of the document (inFlightMode is mapped separately). */
  private static limits(doc: RouteConfig | null): RouteLimits {
    const src = doc?.limits ?? {};
    const keys = Object.keys(RouteSettingsRules.emptyLimits) as (keyof RouteLimits)[];
    return Object.fromEntries(keys.map((k) => [k, src[k] ?? null])) as RouteLimits;
  }

  /** Numeric limits plus the in-flight mode; auto is the default, so only fixed is written. */
  private static limitsDoc(limits: Partial<RouteLimits>, mode: InFlightMode | undefined): NonNullable<RouteConfig['limits']> {
    const out: NonNullable<RouteConfig['limits']> = {};
    (Object.keys(limits) as (keyof RouteLimits)[]).forEach((k) => RouteSettingsMapper.setOptional(out, k, limits[k] ?? undefined));
    RouteSettingsMapper.setOptional(out, 'inFlightMode', mode === InFlightMode.Fixed ? mode : undefined);
    return out;
  }

  private static setOptional<T extends object, K extends keyof T>(target: T, key: K, value: T[K] | undefined): void {
    if (value === undefined) delete target[key];
    else target[key] = value;
  }
}
