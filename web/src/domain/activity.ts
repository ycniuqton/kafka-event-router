/** Verbs shown in the dashboard "Recent activity" feed. */
export const ActivityVerb = {
  Deployed: 'deployed',
  Edited: 'edited',
  Paused: 'paused',
  Resumed: 'resumed',
  RolledBack: 'rolled back',
  Created: 'created',
} as const;
export type ActivityVerb = (typeof ActivityVerb)[keyof typeof ActivityVerb];

/** One activity line: "<actor> <verb> <routeName> <detail>" (design `ACTIVITY`). */
export type Activity = {
  id: string;
  actor: string;
  verb: ActivityVerb;
  routeName: string;
  detail: string;
  at: string;
};

/** The dashboard feed shows the last 10 route changes (spec §4). */
export const ActivityFeed = { recentLimit: 10 } as const;
