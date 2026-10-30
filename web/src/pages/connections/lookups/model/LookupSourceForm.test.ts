import { describe, expect, it } from 'vitest';
import { LookupSecretField, LookupSourceDefaults, LookupSourceKind } from '../../../../domain';
import { LookupDraftPatch, LookupErrorSteps, LookupFormStep, LookupSecretView } from './LookupSourceForm';
import { LookupSourceRows } from './LookupSourceRows';
import { NumberText } from './NumberText';
import { LOOKUP_SOURCES } from '../../../../mocks';

describe('lookup source form model', () => {
  it('routes each error pointer to the step that shows it', () => {
    const errors = { '/limits/burst': 'x', '/secrets/token': 'y' };
    expect(LookupErrorSteps.stepOf('/name')).toBe(LookupFormStep.Basics);
    expect(LookupErrorSteps.stepOf('/http/baseUrl')).toBe(LookupFormStep.Kind);
    expect(LookupErrorSteps.firstStep(errors)).toBe(LookupFormStep.Kind);
    expect(LookupErrorSteps.onStep(errors, LookupFormStep.Limits)).toEqual({ '/limits/burst': 'x' });
    expect(LookupErrorSteps.firstStep({})).toBeNull();
  });

  it('shows stored secrets as write-only until a replacement is typed', () => {
    const d = LookupSourceDefaults.draft();
    expect(LookupSecretView.of(d, [LookupSecretField.Token], LookupSecretField.Token)).toEqual({ value: '', stored: true });
    const replacing = LookupDraftPatch.secret(d, LookupSecretField.Token, '');
    expect(LookupSecretView.of(replacing, [LookupSecretField.Token], LookupSecretField.Token)).toEqual({ value: '', stored: false });
  });

  it('patches nested settings immutably', () => {
    const d = LookupSourceDefaults.draft();
    const next = LookupDraftPatch.httpAuth(LookupDraftPatch.http(d, { baseUrl: 'https://x' }), { username: 'u' });
    expect(next.http.baseUrl).toBe('https://x');
    expect(next.http.auth.username).toBe('u');
    expect(d.http.baseUrl).toBe('');
    expect(LookupDraftPatch.limits(d, { burst: 5 }).limits.burst).toBe(5);
  });

  it('parses number inputs (blank → NaN, shown empty)', () => {
    expect(NumberText.of('12')).toBe(12);
    expect(Number.isNaN(NumberText.of(' '))).toBe(true);
    expect(NumberText.show(Number.NaN)).toBe('');
  });

  it('filters rows by kind and text and builds the target column', () => {
    expect(LookupSourceRows.filter(LOOKUP_SOURCES, { kind: LookupSourceKind.Redis, text: '' }).map((s) => s.name)).toEqual(['feature-flags']);
    expect(LookupSourceRows.filter(LOOKUP_SOURCES, { kind: null, text: 'region' }).map((s) => s.name)).toEqual(['price-list']);
    const row = LookupSourceRows.build(LOOKUP_SOURCES[0], new Date('2026-10-13T10:00:00Z'));
    expect(row).toMatchObject({ kindLabel: 'HTTP', target: 'https://crm.internal/api/v2', statusLabel: 'Healthy' });
  });
});
