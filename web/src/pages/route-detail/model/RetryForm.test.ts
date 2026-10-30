import { describe, expect, it } from 'vitest';
import { DeliveryMode, ErrorAction, InFlightMode, RouteSettingsRules, type RouteSettings } from '../../../domain';
import { RouteSettingsMapper } from '../../../api/http/routes/RouteSettingsMapper';
import { RetryForm, SettingsField, SettingsForm } from './SettingsForm';

const settings: RouteSettings = {
  name: 'Orders',
  description: '',
  tags: [],
  replicas: 2,
  errorPolicy: { ...RouteSettingsRules.defaultErrorPolicy, retry: { enabled: true, maxAttempts: 3, baseDelayMs: 500, maxDelayMs: 4000 } },
  deliveryMode: DeliveryMode.AtLeastOnce,
  inFlightMode: InFlightMode.Auto,
  limits: RouteSettingsRules.emptyLimits,
};

describe('retry policy settings', () => {
  it('round-trips through the form and validates bounds', () => {
    const values = SettingsForm.values(settings);
    expect(values.retry).toEqual({ enabled: true, maxAttempts: '3', baseDelayMs: '500', maxDelayMs: '4000' });
    expect(SettingsForm.patch(values).errorPolicy?.retry).toEqual(settings.errorPolicy.retry);
    const bad = { ...values, retry: { ...values.retry, maxAttempts: '12', maxDelayMs: '100' } };
    const errors = SettingsForm.validate(bad);
    expect(errors[SettingsField.RetryAttempts]).toBeTruthy();
    expect(errors[SettingsField.RetryMax]).toBeTruthy();
    expect(RetryForm.patch({ ...values.retry, enabled: false })).toBeNull();
    expect(SettingsForm.values({ ...settings, errorPolicy: RouteSettingsRules.defaultErrorPolicy }).retry.enabled).toBe(false);
  });

  it('writes errorPolicy.retry into the route document and drops it when disabled', () => {
    const doc = { apiVersion: 'router/v1', name: 'o', source: { connectionId: 'c', format: { type: 'json' } }, branches: [{ id: 'main', destination: { connectionId: 'd', topic: 't' } }], errorPolicy: { onError: ErrorAction.DeadLetter } };
    const apply = (draft: unknown, errorPolicy: RouteSettings['errorPolicy']) => RouteSettingsMapper.toBody({ draft } as never, { errorPolicy }).draft as typeof doc & { errorPolicy?: { retry?: { enabled: boolean } } };
    const withRetry = apply(doc, settings.errorPolicy);
    expect(withRetry.errorPolicy?.retry).toEqual({ enabled: true, maxAttempts: 3, baseDelayMs: 500, maxDelayMs: 4000 });
    const without = apply(withRetry, { ...settings.errorPolicy, retry: null });
    expect(without.errorPolicy && 'retry' in without.errorPolicy).toBe(false);
    const kept = apply(withRetry, RouteSettingsRules.defaultErrorPolicy);
    expect(kept.errorPolicy?.retry?.enabled).toBe(true);
  });
});
