import { Tone } from './tone';

/** What an alert rule watches (server `alert_rule_kind`). */
export const AlertRuleKind = {
  LagAbove: 'lag_above',
  ErrorRateAbove: 'error_rate_above',
  ThroughputZero: 'throughput_zero',
  MatchRateChange: 'match_rate_change',
} as const;
export type AlertRuleKind = (typeof AlertRuleKind)[keyof typeof AlertRuleKind];

/** Where a notification goes. */
export const ChannelType = { Email: 'email', Slack: 'slack', Teams: 'teams', Webhook: 'webhook' } as const;
export type ChannelType = (typeof ChannelType)[keyof typeof ChannelType];

/** Lifecycle of one alert (firing → resolved). */
export const AlertState = { Firing: 'firing', Resolved: 'resolved' } as const;
export type AlertState = (typeof AlertState)[keyof typeof AlertState];

/** A channel as the API returns it: secret URLs are never sent back, only whether one is stored. */
export type ChannelView = { type: ChannelType; target?: string; secretSet: boolean };
/** A channel as the client sends it: `secret` is write-only and omitted to keep the stored URL. */
export type ChannelInput = { type: ChannelType; target?: string; secret?: string };

export type AlertRule = {
  id: string;
  routeId: string;
  name: string;
  kind: AlertRuleKind;
  threshold: number;
  windowMinutes: number;
  channels: ChannelView[];
  enabled: boolean;
  version: number;
};

export type AlertRuleDraft = Omit<AlertRule, 'id' | 'channels' | 'version'> & { channels: ChannelInput[] };

/** The rule being saved: a new draft, or an update of `id` at `version` (If-Match). */
export type AlertRuleSave = { draft: AlertRuleDraft; existing?: { id: string; version: number } };

/** Result of the "Send test" button for one channel. */
export type ChannelTestResult = { channel: ChannelType; ok: boolean; error?: string };

type KindPresentation = { label: string; thresholdLabel: string; unit: string; defaultThreshold: number; min: number; max: number; hasThreshold: boolean };

/** Form labels and limits per kind (same limits as the server's `KindLimits`). */
export const AlertRuleKindMeta: Record<AlertRuleKind, KindPresentation> = {
  [AlertRuleKind.LagAbove]: { label: 'Lag above', thresholdLabel: 'Lag (messages)', unit: 'msgs', defaultThreshold: 20000, min: 1, max: 1e12, hasThreshold: true },
  [AlertRuleKind.ErrorRateAbove]: { label: 'Error rate above', thresholdLabel: 'Error rate (%)', unit: '%', defaultThreshold: 5, min: 0.01, max: 100, hasThreshold: true },
  [AlertRuleKind.ThroughputZero]: { label: 'Throughput is 0', thresholdLabel: '', unit: '', defaultThreshold: 0, min: 0, max: 0, hasThreshold: false },
  [AlertRuleKind.MatchRateChange]: { label: 'Match rate changes', thresholdLabel: 'Change (points)', unit: 'pts', defaultThreshold: 20, min: 1, max: 100, hasThreshold: true },
};

/** Channel labels and what their target field means. */
export const ChannelTypeMeta: Record<ChannelType, { label: string; usesSecret: boolean; placeholder: string }> = {
  [ChannelType.Email]: { label: 'Email', usesSecret: false, placeholder: 'ops@example.com, oncall@example.com' },
  [ChannelType.Slack]: { label: 'Slack', usesSecret: true, placeholder: 'https://hooks.slack.com/services/…' },
  [ChannelType.Teams]: { label: 'Microsoft Teams', usesSecret: true, placeholder: 'https://outlook.office.com/webhook/…' },
  [ChannelType.Webhook]: { label: 'Webhook', usesSecret: true, placeholder: 'https://example.com/alerts' },
};

/** Tone of a bell row by state. */
export const AlertStateTone: Record<AlertState, Tone> = { [AlertState.Firing]: Tone.Err, [AlertState.Resolved]: Tone.Ok };

/** Form values of one channel row. */
export type ChannelFormValue = { type: ChannelType; target: string; secret: string; secretSet: boolean };

/** Form values of the rule editor (numbers stay strings while typing). */
export type AlertRuleFormValues = {
  name: string;
  kind: AlertRuleKind;
  threshold: string;
  windowMinutes: string;
  enabled: boolean;
  channels: ChannelFormValue[];
};

/** Field keys of form errors (server problem paths map onto them). */
export const AlertRuleField = { Name: 'name', Kind: 'kind', Threshold: 'threshold', Window: 'windowMinutes', Channels: 'channels', Form: 'form' } as const;

/** Validation and conversion of the alert rule form (mirrors the server's RuleValidator). */
export class AlertRuleRules {
  static readonly maxName = 120;
  static readonly window = { min: 1, max: 1440, default: 10 } as const;
  static readonly maxChannels = 10;
  static readonly maxRecipients = 20;
  static readonly recipientSeparator = ',';
  static readonly emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  static readonly urlPattern = /^https?:\/\/[^\s/]+/;

  static empty(): AlertRuleFormValues {
    const kind = AlertRuleKind.LagAbove;
    return { name: '', kind, threshold: String(AlertRuleKindMeta[kind].defaultThreshold), windowMinutes: String(AlertRuleRules.window.default), enabled: true, channels: [] };
  }

  static fromRule(rule: AlertRule): AlertRuleFormValues {
    return {
      name: rule.name,
      kind: rule.kind,
      threshold: String(rule.threshold),
      windowMinutes: String(rule.windowMinutes),
      enabled: rule.enabled,
      channels: rule.channels.map((c) => ({ type: c.type, target: c.target ?? '', secret: '', secretSet: c.secretSet })),
    };
  }

  static channel(type: ChannelType): ChannelFormValue {
    return { type, target: '', secret: '', secretSet: false };
  }

  /** Returns field → message; empty when valid. Channel errors use `channels.<i>`. */
  static validate(v: AlertRuleFormValues): Record<string, string> {
    const errors: Record<string, string> = {};
    const meta = AlertRuleKindMeta[v.kind];
    const threshold = Number(v.threshold);
    const window = Number(v.windowMinutes);
    if (!v.name.trim() || v.name.trim().length > AlertRuleRules.maxName) errors[AlertRuleField.Name] = 'Give the rule a name of up to 120 characters.';
    if (meta.hasThreshold && (v.threshold.trim() === '' || !Number.isFinite(threshold) || threshold < meta.min || threshold > meta.max)) {
      errors[AlertRuleField.Threshold] = `Enter a value between ${meta.min} and ${meta.max}.`;
    }
    if (!Number.isInteger(window) || window < AlertRuleRules.window.min || window > AlertRuleRules.window.max) {
      errors[AlertRuleField.Window] = 'The window must be between 1 and 1440 minutes.';
    }
    if (v.channels.length > AlertRuleRules.maxChannels) errors[AlertRuleField.Channels] = 'A rule can notify at most 10 channels.';
    v.channels.forEach((c, i) => {
      const message = AlertRuleRules.channelError(c);
      if (message) errors[`${AlertRuleField.Channels}.${i}`] = message;
    });
    return errors;
  }

  static channelError(c: ChannelFormValue): string | null {
    if (!ChannelTypeMeta[c.type].usesSecret) {
      const list = AlertRuleRules.recipients(c.target);
      const valid = list.length > 0 && list.length <= AlertRuleRules.maxRecipients && list.every((a) => AlertRuleRules.emailPattern.test(a));
      return valid ? null : 'Enter 1 to 20 email addresses separated by commas.';
    }
    if (!c.secret.trim()) return c.secretSet ? null : 'Enter the webhook URL for this channel.';
    return AlertRuleRules.urlPattern.test(c.secret.trim()) ? null : 'Enter the full https:// URL of the incoming webhook.';
  }

  static recipients(target: string): string[] {
    return target.split(AlertRuleRules.recipientSeparator).map((s) => s.trim()).filter(Boolean);
  }

  static toDraft(routeId: string, v: AlertRuleFormValues): AlertRuleDraft {
    const meta = AlertRuleKindMeta[v.kind];
    return {
      routeId,
      name: v.name.trim(),
      kind: v.kind,
      threshold: meta.hasThreshold ? Number(v.threshold) : meta.defaultThreshold,
      windowMinutes: Number(v.windowMinutes),
      enabled: v.enabled,
      channels: v.channels.map((c) => AlertRuleRules.channelInput(c)),
    };
  }

  private static channelInput(c: ChannelFormValue): ChannelInput {
    if (!ChannelTypeMeta[c.type].usesSecret) return { type: c.type, target: AlertRuleRules.recipients(c.target).join(AlertRuleRules.recipientSeparator) };
    return c.secret.trim() ? { type: c.type, secret: c.secret.trim() } : { type: c.type };
  }

  /** "Lag above 20,000 msgs for 10 min" — the rule in one line. */
  static summary(rule: Pick<AlertRule, 'kind' | 'threshold' | 'windowMinutes'>): string {
    const meta = AlertRuleKindMeta[rule.kind];
    const value = meta.hasThreshold ? ` ${rule.threshold.toLocaleString('en-US')} ${meta.unit}` : '';
    return `${meta.label}${value} for ${rule.windowMinutes} min`;
  }
}
