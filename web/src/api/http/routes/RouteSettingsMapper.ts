import type { RouteConfig } from '../../generated';
import { DeliveryMode, ErrorAction, RouteSettingsRules, type RouteErrorPolicy, type RouteLimits, type RouteSettings, type RouteSettingsPatch } from '../../../domain';
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
    if (doc && (patch.errorPolicy || patch.limits || patch.deliveryMode)) body.draft = RouteSettingsMapper.applyToDocument(doc, patch);
    return body;
  }

  static document(dto: RouteDetailDto): RouteConfig | null {
    return dto.draft ?? dto.active ?? null;
  }

  private static applyToDocument(doc: RouteConfig, patch: RouteSettingsPatch): RouteConfig {
    const next: RouteConfig = { ...doc };
    if (patch.errorPolicy) next.errorPolicy = RouteSettingsMapper.errorPolicyDoc(patch.errorPolicy, doc.errorPolicy);
    if (patch.limits) next.limits = RouteSettingsMapper.limitsDoc(patch.limits);
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
    };
  }

  private static errorPolicyDoc(policy: RouteErrorPolicy, base: RouteConfig['errorPolicy']): NonNullable<RouteConfig['errorPolicy']> {
    const out: NonNullable<RouteConfig['errorPolicy']> = {
      ...base,
      onError: policy.onError,
    };
    RouteSettingsMapper.setOptional(out, 'dlqConnectionId', policy.dlqConnectionId || undefined);
    RouteSettingsMapper.setOptional(out, 'dlqTopic', policy.dlqTopic || undefined);
    RouteSettingsMapper.setOptional(out, 'maxRetries', policy.maxRetries ?? undefined);
    return out;
  }

  private static limits(doc: RouteConfig | null): Partial<RouteLimits> {
    return { ...(doc?.limits ?? {}) };
  }

  private static limitsDoc(limits: RouteLimits): NonNullable<RouteConfig['limits']> {
    const out: NonNullable<RouteConfig['limits']> = {};
    (Object.keys(limits) as (keyof RouteLimits)[]).forEach((k) => RouteSettingsMapper.setOptional(out, k, limits[k] ?? undefined));
    return out;
  }

  private static setOptional<T extends object, K extends keyof T>(target: T, key: K, value: T[K] | undefined): void {
    if (value === undefined) delete target[key];
    else target[key] = value;
  }
}
