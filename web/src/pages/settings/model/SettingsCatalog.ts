import { Role, Tone } from '../../../domain';

/** What each role can do (spec §11.2), shown read-only in Settings. */
export const RoleCapabilities: Record<Role, string> = {
  [Role.Viewer]: 'See routes, metrics, history; samples are masked',
  [Role.Editor]: 'Create and edit routes and transforms in their teams; deploy to non-production connections',
  [Role.Approver]: 'Approve deploys to connections marked "production"',
  [Role.Admin]: 'Manage connections, credentials, teams, users and settings',
};

/** Choice of the "phase tags" control. */
export const PhaseTagChoice = { Show: 'show', Hide: 'hide' } as const;
export type PhaseTagChoice = (typeof PhaseTagChoice)[keyof typeof PhaseTagChoice];

/** Alert delivery channels (spec §11.4). */
export const ChannelKind = { Email: 'email', Slack: 'slack', Teams: 'teams', Webhook: 'webhook' } as const;
export type ChannelKind = (typeof ChannelKind)[keyof typeof ChannelKind];

export type NotificationChannel = { kind: ChannelKind; name: string; target: string | null };

/** Channel state presentation. */
export const ChannelStatus = {
  connected: { label: 'Connected', tone: Tone.Ok },
  notSetUp: { label: 'Not set up', tone: Tone.Off },
} as const;

/**
 * Static notification channels until a notifications API exists.
 * Sending a test only shows the toast.
 */
export class NotificationChannelCatalog {
  static readonly channels: readonly NotificationChannel[] = [
    { kind: ChannelKind.Email, name: 'Email', target: 'platform-oncall@vcc.internal' },
    { kind: ChannelKind.Slack, name: 'Slack', target: '#event-router-alerts' },
    { kind: ChannelKind.Teams, name: 'Microsoft Teams', target: null },
    { kind: ChannelKind.Webhook, name: 'Webhook', target: null },
  ];
  static readonly testSentPrefix = 'Test notification sent · ';
}

