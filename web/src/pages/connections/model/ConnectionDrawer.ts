import { TestStepState, Tone } from '../../../domain';

/** Which side panel the connections screen shows (the test drawer is driven by `?test=`). */
export const ConnectionPanelMode = { Browse: 'browse', Create: 'create', Edit: 'edit' } as const;
export type ConnectionPanelMode = (typeof ConnectionPanelMode)[keyof typeof ConnectionPanelMode];

export type ConnectionPanel = { mode: ConnectionPanelMode; connectionId: string | null };

/** Drawer titles from the design. */
export const ConnectionDrawerTitle = {
  testPrefix: 'Test · ',
  topicsPrefix: 'Topics · ',
  create: 'New connection',
  editPrefix: 'Edit · ',
} as const;

/** Icon, tone and pulse of each checklist state (design `testSteps`). */
export const TestStepPresentation: Record<TestStepState, { icon: string; tone: Tone; pulse: boolean; dim: boolean }> = {
  [TestStepState.Passed]: { icon: '✓', tone: Tone.Ok, pulse: false, dim: false },
  [TestStepState.Failed]: { icon: '✕', tone: Tone.Err, pulse: false, dim: false },
  [TestStepState.Running]: { icon: '●', tone: Tone.Accent, pulse: true, dim: false },
  [TestStepState.Pending]: { icon: '○', tone: Tone.Muted, pulse: false, dim: false },
  [TestStepState.Skipped]: { icon: '○', tone: Tone.Muted, pulse: false, dim: true },
};

/** Topic peek: how many events to fetch and how much of each to show on one line. */
export const TopicPeekConfig = { count: 20, previewChars: 140 } as const;

/** Topic peek messages shown when sampling fails, and the masked badge. */
export const TopicPeekText = {
  failed: "Couldn't read this topic.",
  retry: 'Retry',
  masked: 'masked',
  maskedSensitive: 'Values are masked: this connection is marked sensitive.',
  maskedRole: 'Values are masked for your role.',
} as const;

/** Whether peeked events are masked (live-preview masks Viewers always, everyone on sensitive connections). */
export class PeekMasking {
  static masked(sensitive: boolean, canReadRaw: boolean): boolean {
    return sensitive || !canReadRaw;
  }

  static reason(sensitive: boolean): string {
    return sensitive ? TopicPeekText.maskedSensitive : TopicPeekText.maskedRole;
  }
}

/** Texts of the demo-connection badge. */
export const DemoBadgeText = {
  label: 'Demo',
  title: 'In-memory demo cluster: order-events gets a new sample order every second. Nothing leaves the router.',
} as const;
