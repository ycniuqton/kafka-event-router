import { describe, expect, it } from 'vitest';
import { AlertRuleKind, AlertRuleKindMeta, AlertRuleRules } from './alertRule';
import { DeadLetterKind, type DeadLetter } from './deadLetter';
import { DeadLetterClasses, DeadLetterErrorClass, DeadLetterErrorClassMeta } from './deadLetterClass';
import { DiffOp, RecordEditDraft, ReplayRate, StructuralDiff, EditBlock } from './deadLetterEdit';
import { CauseFingerprint, DeadLetterGroups, MessageNormalizer } from './deadLetterGroups';
import { RetryPolicyRules } from './retryPolicy';
import { Tone } from './tone';

function row(id: string, message: string, failedAt: string, extra: Partial<DeadLetter> = {}): DeadLetter {
  return { id, failedAt, kind: DeadLetterKind.TransformError, message, stack: '', offset: 1, partition: 0, routeId: 'r1', step: 'Step 1', sampleId: -1, ...extra };
}

describe('dead-letter classification', () => {
  it('classifies every kind like the server table', () => {
    Object.values(DeadLetterKind).forEach((k) => expect(DeadLetterClasses.order).toContain(DeadLetterClasses.ofKind(k)));
    expect(DeadLetterClasses.ofKind(DeadLetterKind.TransformError)).toBe(DeadLetterErrorClass.Data);
    expect(DeadLetterClasses.ofKind(DeadLetterKind.InvalidOutput)).toBe(DeadLetterErrorClass.Data);
    expect(DeadLetterClasses.ofKind(DeadLetterKind.Timeout)).toBe(DeadLetterErrorClass.Transient);
    expect(DeadLetterClasses.ofKind(DeadLetterKind.LookupUnavailable)).toBe(DeadLetterErrorClass.Transient);
    expect(DeadLetterClasses.ofKind(DeadLetterKind.SandboxCrash)).toBe(DeadLetterErrorClass.Router);
    expect(DeadLetterClasses.ofKind(DeadLetterKind.Unreadable)).toBe(DeadLetterErrorClass.Router);
    expect(DeadLetterClasses.of({ kind: DeadLetterKind.TransformError, errorClass: DeadLetterErrorClass.Router })).toBe(DeadLetterErrorClass.Router);
  });

  it('has badge meta per class and counts classes from kind counts', () => {
    expect(DeadLetterErrorClassMeta[DeadLetterErrorClass.Router]).toMatchObject({ label: 'Router bug', tone: Tone.Err });
    expect(DeadLetterErrorClassMeta[DeadLetterErrorClass.Data].tone).toBe(Tone.Warn);
    expect(DeadLetterErrorClassMeta[DeadLetterErrorClass.Transient].tone).toBe(Tone.Info);
    const counts = DeadLetterClasses.counts([
      { kind: DeadLetterKind.TransformError, count: 3 },
      { kind: DeadLetterKind.SandboxCrash, count: 1 },
      { kind: DeadLetterKind.Timeout, count: 2 },
    ]);
    expect(counts).toEqual({ data: 3, router: 1, transient: 2 });
    expect(DeadLetterClasses.kindsOf(DeadLetterErrorClass.Transient)).toContain(DeadLetterKind.ProduceRetriable);
    expect(DeadLetterClasses.valid('router')).toBe(true);
    expect(DeadLetterClasses.valid('nope')).toBe(false);
  });
});

describe('cause fingerprinting', () => {
  it('normalizes ids, numbers, offsets, positions and timestamps', () => {
    expect(MessageNormalizer.normalize('Unknown customer cus_8812 at offset 120050')).toBe('Unknown customer <id> at offset <n>');
    expect(MessageNormalizer.normalize('order 3fa85f64-5717-4562-b3fc-2c963f66afa6 failed at 2026-10-13T09:41:07Z')).toBe('order <uuid> failed at <ts>');
    expect(MessageNormalizer.normalize('at transform (step.js:7:22)  took   120 ms')).toBe('at transform (step.js:<n>:<n>) took <n> ms');
    expect(MessageNormalizer.normalize('hash deadbeef42 p3@884162')).toBe('hash <hex> p<n>@<n>');
    expect(MessageNormalizer.normalize("Cannot read properties of undefined (reading 'reduce')")).toBe("Cannot read properties of undefined (reading 'reduce')");
  });

  it('gives the same cause the same fingerprint and different steps or routes another one', () => {
    const a = CauseFingerprint.ofRow(row('1', 'Unknown customer cus_1 at offset 5', 'x'));
    const b = CauseFingerprint.ofRow(row('2', 'Unknown customer cus_77 at offset 912', 'x'));
    expect(a).toBe(b);
    expect(a).toMatch(/^[0-9a-f]{16}$/);
    expect(CauseFingerprint.ofRow(row('3', 'Unknown customer cus_1 at offset 5', 'x', { step: 'Step 2' }))).not.toBe(a);
    expect(CauseFingerprint.ofRow(row('4', 'Unknown customer cus_1 at offset 5', 'x', { routeId: 'r2' }))).not.toBe(a);
    expect(CauseFingerprint.ofRow(row('5', 'm', 'x', { fingerprint: 'server' }))).toBe('server');
  });

  it('groups rows by count with first/last seen, sample, versions and hourly trend', () => {
    const now = new Date('2026-10-13T10:00:00Z');
    const rows = [
      row('a', 'missing field total in ord_1', '2026-10-13T09:30:00Z', { versionNo: 5 }),
      row('b', 'missing field total in ord_2', '2026-10-13T08:10:00Z', { versionNo: 4 }),
      row('c', 'boom', '2026-10-13T09:50:00Z', { kind: DeadLetterKind.SandboxCrash }),
    ];
    const page = DeadLetterGroups.build(rows, now, () => 'Orders');
    expect(page.items.map((g) => g.count)).toEqual([2, 1]);
    const top = page.items[0];
    expect(top).toMatchObject({ pattern: 'missing field total in <id>', firstSeenAt: '2026-10-13T08:10:00Z', lastSeenAt: '2026-10-13T09:30:00Z', versions: [4, 5], routeName: 'Orders' });
    expect(top.sample.id).toBe('a');
    expect(top.trend.counts).toHaveLength(DeadLetterGroups.buckets);
    expect(top.trend.counts.reduce((n, c) => n + c, 0)).toBe(2);
    expect(page.byClass).toEqual({ data: 2, router: 1 });
    expect(DeadLetterGroups.bars({ since: '', bucketSeconds: 3600, counts: [0, 2, 4] })).toEqual([0, 50, 100]);
  });
});

describe('record edits', () => {
  const original = { key: 'ord_1', value: { items: null, total: 3 }, headers: [{ key: 'trace', value: 'ab' }], rawBytes: false };

  it('validates JSON and headers and returns only changed members', () => {
    const texts = RecordEditDraft.texts(original);
    expect(RecordEditDraft.validate(texts)).toEqual({});
    expect(RecordEditDraft.edit(original, texts)).toBeNull();
    const broken = { ...texts, value: '{"items": [' , headers: '[{"key": 1}]' };
    expect(Object.keys(RecordEditDraft.validate(broken)).sort()).toEqual(['headers', 'value']);
    const changed = { ...texts, value: JSON.stringify({ items: [], total: 3 }) };
    expect(RecordEditDraft.edit(original, changed)).toEqual({ value: { items: [], total: 3 } });
  });

  it('blocks raw bytes and schema payloads', () => {
    expect(RecordEditDraft.blocked(original)).toBeNull();
    expect(RecordEditDraft.blocked({ ...original, rawBytes: true })).toBe(EditBlock.RawBytes);
    expect(RecordEditDraft.blocked({ ...original, schema: { format: 'avro' } })).toBe(EditBlock.Schema);
    expect(RecordEditDraft.blocked(null)).toBe(EditBlock.Missing);
  });

  it('diffs by JSON path without values', () => {
    const diff = StructuralDiff.of(original, { value: { items: [1], note: 'x' }, headers: [{ key: 'trace', value: 'cd' }, { key: 'a/b', value: '1' }] });
    expect(diff).toEqual([
      { path: '/value/items', op: DiffOp.Replace },
      { path: '/value/total', op: DiffOp.Remove },
      { path: '/value/note', op: DiffOp.Add },
      { path: '/headers/trace', op: DiffOp.Replace },
      { path: '/headers/a~1b', op: DiffOp.Add },
    ]);
  });

  it('bounds the bulk replay rate', () => {
    expect(ReplayRate.parse('50')).toBe(50);
    expect(ReplayRate.parse('0')).toBeNull();
    expect(ReplayRate.parse('1001')).toBeNull();
    expect(ReplayRate.parse('2.5')).toBeNull();
    expect(ReplayRate.seconds(120, 50)).toBe(3);
  });
});

describe('retry policy', () => {
  it('computes the capped exponential schedule', () => {
    const p = { enabled: true, maxAttempts: 5, baseDelayMs: 1000, maxDelayMs: 5000 };
    expect(RetryPolicyRules.delays(p)).toEqual([1000, 2000, 4000, 5000, 5000]);
    expect(RetryPolicyRules.schedule(p)).toBe('1 s → 2 s → 4 s → 5 s → 5 s');
    expect(RetryPolicyRules.duration(90_000)).toBe('1.5 min');
    expect(RetryPolicyRules.duration(7_200_000)).toBe('2 h');
  });

  it('validates bounds only when enabled', () => {
    expect(RetryPolicyRules.validate({ enabled: false, maxAttempts: 0, baseDelayMs: 0, maxDelayMs: 0 })).toEqual({});
    const errors = RetryPolicyRules.validate({ enabled: true, maxAttempts: 11, baseDelayMs: 2000, maxDelayMs: 1000 });
    expect(Object.keys(errors).sort()).toEqual(['maxAttempts', 'maxDelayMs']);
    expect(RetryPolicyRules.validate({ ...RetryPolicyRules.defaults, enabled: true })).toEqual({});
  });
});

describe('router-caused dead-letter alert kind', () => {
  it('defaults to more than 0 events in 15 minutes', () => {
    const meta = AlertRuleKindMeta[AlertRuleKind.RouterCausedDeadLetters];
    expect(meta).toMatchObject({ label: 'Router-caused dead letters', defaultThreshold: 0, min: 0, defaultWindow: 15 });
    const values = AlertRuleRules.empty();
    expect(AlertRuleRules.kindPatch(AlertRuleKind.RouterCausedDeadLetters, values)).toEqual({ kind: AlertRuleKind.RouterCausedDeadLetters, threshold: '0', windowMinutes: '15' });
    expect(AlertRuleRules.kindPatch(AlertRuleKind.LagAbove, { ...values, windowMinutes: '30' }).windowMinutes).toBe('30');
    const form = { ...values, name: 'Router bugs', ...AlertRuleRules.kindPatch(AlertRuleKind.RouterCausedDeadLetters, values) };
    expect(AlertRuleRules.validate(form)).toEqual({});
    expect(AlertRuleRules.toDraft('r1', form)).toMatchObject({ kind: 'router_caused_dead_letters', threshold: 0, windowMinutes: 15 });
  });
});
