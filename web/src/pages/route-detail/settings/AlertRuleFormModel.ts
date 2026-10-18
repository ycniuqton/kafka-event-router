import { ApiError, FieldErrorMapper, ProblemKind } from '../../../api/http/core';
import { AlertRuleField } from '../../../domain';

/** Messages of the alert rules panel. */
export const AlertRulesText = {
  title: 'Alert rules',
  meta: 'Notify when this route needs attention',
  empty: 'No alert rules yet. Add one to get notified by email, Slack, Teams or a webhook.',
  add: '+ Add rule',
  conflict: 'Someone else changed this rule. Reload the panel, then save again.',
  deleteConfirm: 'Delete this alert rule and its history?',
  loadFailed: 'Could not load alert rules',
  disabled: 'Disabled',
  noChannels: 'No channels · shows in the bell only',
  testSent: 'Test sent',
} as const;

/** Maps API failures of the rule form onto its fields (`/channels/2/secret` → `channels.2`). */
export class AlertRuleFailure {
  private static readonly mapper = new FieldErrorMapper();
  private static readonly channelField = /^channels\.(\d+)(\..*)?$/;

  static of(error: unknown): Record<string, string> {
    if (!(error instanceof ApiError)) return { [AlertRuleField.Form]: (error as Error).message };
    if (error.problemKind === ProblemKind.PreconditionFailed || error.problemKind === ProblemKind.Conflict) return { [AlertRuleField.Form]: AlertRulesText.conflict };
    const out: Record<string, string> = {};
    for (const [field, message] of Object.entries(AlertRuleFailure.mapper.map(error))) {
      const channel = AlertRuleFailure.channelField.exec(field);
      out[channel ? `${AlertRuleField.Channels}.${channel[1]}` : field] = message;
    }
    return Object.keys(out).length ? out : { [AlertRuleField.Form]: error.message };
  }
}

/** Identity of the panel in routes' Settings tab slot. */
export class AlertRulesSettingsPanel {
  static readonly id = 'monitoring.alert-rules';
  static readonly order = 100;
}
