import { describe, expect, it } from 'vitest';
import { AlertRuleField, AlertRuleKind, AlertRuleRules, ChannelType, type AlertRuleFormValues } from './alertRule';

const form = (over: Partial<AlertRuleFormValues> = {}): AlertRuleFormValues => ({ ...AlertRuleRules.empty(), name: 'Lag high', ...over });

describe('AlertRuleRules', () => {
  it('accepts a valid lag rule and builds the draft', () => {
    const v = form({ channels: [{ type: ChannelType.Email, target: ' a@x.io ,b@x.io ', secret: '', secretSet: false }] });
    expect(AlertRuleRules.validate(v)).toEqual({});
    expect(AlertRuleRules.toDraft('r1', v)).toEqual({
      routeId: 'r1', name: 'Lag high', kind: AlertRuleKind.LagAbove, threshold: 20000, windowMinutes: 10, enabled: true,
      channels: [{ type: ChannelType.Email, target: 'a@x.io,b@x.io' }],
    });
  });

  it('rejects names, thresholds and windows out of range like the server', () => {
    const errors = AlertRuleRules.validate(form({ name: ' ', kind: AlertRuleKind.ErrorRateAbove, threshold: '101', windowMinutes: '1441' }));
    expect(Object.keys(errors).sort()).toEqual([AlertRuleField.Name, AlertRuleField.Threshold, AlertRuleField.Window].sort());
  });

  it('ignores the threshold of throughput-zero rules and sends 0', () => {
    const v = form({ kind: AlertRuleKind.ThroughputZero, threshold: '' });
    expect(AlertRuleRules.validate(v)).toEqual({});
    expect(AlertRuleRules.toDraft('r1', v).threshold).toBe(0);
  });

  it('requires a webhook URL unless one is already stored, and keeps it when left empty', () => {
    const missing = form({ channels: [AlertRuleRules.channel(ChannelType.Slack)] });
    expect(AlertRuleRules.validate(missing)).toHaveProperty('channels.0');
    const stored = form({ channels: [{ type: ChannelType.Webhook, target: '', secret: '', secretSet: true }] });
    expect(AlertRuleRules.validate(stored)).toEqual({});
    expect(AlertRuleRules.toDraft('r1', stored).channels).toEqual([{ type: ChannelType.Webhook }]);
    const bad = form({ channels: [{ type: ChannelType.Teams, target: '', secret: 'ftp://x', secretSet: false }] });
    expect(AlertRuleRules.validate(bad)['channels.0']).toMatch(/https/);
  });

  it('rejects malformed recipient lists', () => {
    const v = form({ channels: [{ type: ChannelType.Email, target: 'not-an-email', secret: '', secretSet: false }] });
    expect(AlertRuleRules.validate(v)['channels.0']).toMatch(/email/);
  });

  it('summarises a rule in one line', () => {
    expect(AlertRuleRules.summary({ kind: AlertRuleKind.LagAbove, threshold: 20000, windowMinutes: 10 })).toBe('Lag above 20,000 msgs for 10 min');
    expect(AlertRuleRules.summary({ kind: AlertRuleKind.ThroughputZero, threshold: 0, windowMinutes: 5 })).toBe('Throughput is 0 for 5 min');
  });

  it('round-trips a stored rule through the form', () => {
    const rule = { id: 'x', routeId: 'r1', name: 'n', kind: AlertRuleKind.MatchRateChange, threshold: 15, windowMinutes: 30, enabled: false, version: 3,
      channels: [{ type: ChannelType.Webhook, secretSet: true }] };
    const v = AlertRuleRules.fromRule(rule);
    expect(v).toMatchObject({ threshold: '15', windowMinutes: '30', enabled: false });
    expect(AlertRuleRules.toDraft('r1', v)).toMatchObject({ kind: AlertRuleKind.MatchRateChange, threshold: 15, channels: [{ type: ChannelType.Webhook }] });
  });
});
