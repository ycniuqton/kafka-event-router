import { ApiError, FieldErrorMapper, ProblemKind } from '../../../api/http/core';
import { RouteSettingsRules, type DeliveryMode, type ErrorAction, type RouteLimits, type RouteSettings, type RouteSettingsPatch } from '../../../domain';

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
  deliveryMode: DeliveryMode;
  limits: Record<keyof RouteLimits, string>;
};

/** Form field names used for error messages (match the API's JSON Pointers via aliases). */
export const SettingsField = { Name: 'name', Tags: 'tags', Replicas: 'replicas', Retries: 'maxRetries', Form: 'form' } as const;

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
      deliveryMode: s.deliveryMode,
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
      errorPolicy: { onError: v.onError, dlqConnectionId: v.dlqConnectionId, dlqTopic: v.dlqTopic.trim(), maxRetries: RouteSettingsRules.parseLimit(v.maxRetries) },
      deliveryMode: v.deliveryMode,
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
