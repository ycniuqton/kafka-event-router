import { ApiError, FieldErrorMapper, ProblemKind } from '../../../api/http/core';
import { RetryPolicyRules, RouteSettingsRules, type DeliveryMode, type ErrorAction, type InFlightMode, type RetryPolicySettings, type RouteLimits, type RouteSettings, type RouteSettingsPatch } from '../../../domain';

/** Text values of the auto-retry policy (numbers stay text while typing). */
export type RetryFormValues = { enabled: boolean; maxAttempts: string; baseDelayMs: string; maxDelayMs: string };

/** Converts the auto-retry policy between settings and form text. */
export class RetryForm {
  static values(p: RetryPolicySettings | null | undefined): RetryFormValues {
    const v = p ?? RetryPolicyRules.defaults;
    return { enabled: v.enabled, maxAttempts: String(v.maxAttempts), baseDelayMs: String(v.baseDelayMs), maxDelayMs: String(v.maxDelayMs) };
  }

  static policy(v: RetryFormValues): RetryPolicySettings {
    return { enabled: v.enabled, maxAttempts: Number(v.maxAttempts), baseDelayMs: Number(v.baseDelayMs), maxDelayMs: Number(v.maxDelayMs) };
  }

  /** Disabled → null (the document drops `errorPolicy.retry`). */
  static patch(v: RetryFormValues): RetryPolicySettings | null {
    return v.enabled ? RetryForm.policy(v) : null;
  }
}

/** Text values of the Settings form (inputs edit text; numbers are parsed on save). */
export type SettingsFormValues = {
  name: string;
  description: string;
  tags: string;
  replicas: string;
  onError: ErrorAction;
  dlqConnectionId: string;
  dlqTopic: string;
  maxRetries: string;
  retry: RetryFormValues;
  deliveryMode: DeliveryMode;
  inFlightMode: InFlightMode;
  limits: Record<keyof RouteLimits, string>;
};

/** Form field names used for error messages (match the API's JSON Pointers via aliases). */
export const SettingsField = {
  Name: 'name',
  Tags: 'tags',
  Replicas: 'replicas',
  Retries: 'maxRetries',
  RetryAttempts: 'retry.maxAttempts',
  RetryBase: 'retry.baseDelayMs',
  RetryMax: 'retry.maxDelayMs',
  Form: 'form',
} as const;

/** Messages of the Settings tab. */
export const SettingsText = {
  conflict: 'Someone else changed this route. Reload to see their version, then save again.',
  invalidReplicas: `Replicas must be a whole number from ${RouteSettingsRules.minReplicas} to ${RouteSettingsRules.maxReplicas}.`,
  invalidNumber: 'Use a positive whole number, or leave empty for the default.',
  nameRequired: 'Give the route a name.',
} as const;

/** Converts route settings to editable text, validates the edit and builds the PATCH. */
export class SettingsForm {
  private static readonly errors = new FieldErrorMapper({ '/name': SettingsField.Name, '/tags': SettingsField.Tags, '/replicas': SettingsField.Replicas });

  static values(s: RouteSettings): SettingsFormValues {
    const limits = Object.fromEntries(Object.entries(s.limits).map(([k, v]) => [k, v === null ? '' : String(v)])) as Record<keyof RouteLimits, string>;
    return {
      name: s.name,
      description: s.description,
      tags: s.tags.join(`${RouteSettingsRules.tagSeparator} `),
      replicas: String(s.replicas),
      onError: s.errorPolicy.onError,
      dlqConnectionId: s.errorPolicy.dlqConnectionId,
      dlqTopic: s.errorPolicy.dlqTopic,
      maxRetries: s.errorPolicy.maxRetries === null ? '' : String(s.errorPolicy.maxRetries),
      retry: RetryForm.values(s.errorPolicy.retry),
      deliveryMode: s.deliveryMode,
      inFlightMode: s.inFlightMode,
      limits,
    };
  }

  static patch(v: SettingsFormValues): RouteSettingsPatch {
    const limits = Object.fromEntries(Object.entries(v.limits).map(([k, t]) => [k, RouteSettingsRules.parseLimit(t)])) as RouteLimits;
    return {
      name: v.name.trim(),
      description: v.description,
      tags: RouteSettingsRules.parseTags(v.tags),
      replicas: Number(v.replicas),
      errorPolicy: {
        onError: v.onError,
        dlqConnectionId: v.dlqConnectionId,
        dlqTopic: v.dlqTopic.trim(),
        maxRetries: RouteSettingsRules.parseLimit(v.maxRetries),
        retry: RetryForm.patch(v.retry),
      },
      deliveryMode: v.deliveryMode,
      inFlightMode: v.inFlightMode,
      limits,
    };
  }

  /** Client-side checks; keys are form field names (limit keys for limits). */
  static validate(v: SettingsFormValues): Record<string, string> {
    const out: Record<string, string> = {};
    const patch = SettingsForm.patch(v);
    if (!patch.name) out[SettingsField.Name] = SettingsText.nameRequired;
    if (!RouteSettingsRules.validReplicas(patch.replicas ?? 0)) out[SettingsField.Replicas] = SettingsText.invalidReplicas;
    if (!RouteSettingsRules.validLimit(patch.errorPolicy?.maxRetries ?? null)) out[SettingsField.Retries] = SettingsText.invalidNumber;
    const retry = RetryPolicyRules.validate(RetryForm.policy(v.retry));
    const retryFields = { maxAttempts: SettingsField.RetryAttempts, baseDelayMs: SettingsField.RetryBase, maxDelayMs: SettingsField.RetryMax } as const;
    (Object.keys(retryFields) as (keyof typeof retryFields)[]).forEach((k) => {
      if (retry[k]) out[retryFields[k]] = retry[k];
    });
    Object.entries(patch.limits ?? {}).forEach(([k, n]) => {
      if (!RouteSettingsRules.validLimit(n)) out[k] = SettingsText.invalidNumber;
    });
    return out;
  }

  /** Server failure → field messages, with a form-level message for conflicts and other errors. */
  static failure(error: unknown): Record<string, string> {
    if (!(error instanceof ApiError)) return { [SettingsField.Form]: (error as Error).message };
    if (error.problemKind === ProblemKind.Conflict || error.problemKind === ProblemKind.PreconditionFailed) return { [SettingsField.Form]: SettingsText.conflict };
    const fields = SettingsForm.errors.map(error);
    return Object.keys(fields).length ? fields : { [SettingsField.Form]: error.message };
  }
}
