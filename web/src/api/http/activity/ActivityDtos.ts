/** Paths and parameters of the activity feed (relative to `/api/v1`). */
export const ActivityEndpoint = { collection: '/activity' } as const;
export const ActivityParam = { limit: 'limit' } as const;

/** Verbs the API sends for route changes. */
export const ActivityVerbDto = {
  Created: 'created',
  Edited: 'edited',
  Deployed: 'deployed',
  Paused: 'paused',
  Resumed: 'resumed',
  RolledBack: 'rolled_back',
} as const;
export type ActivityVerbDto = (typeof ActivityVerbDto)[keyof typeof ActivityVerbDto];

/** One `GET /activity` item. */
export type ActivityItemDto = {
  id: string;
  actor: string;
  verb: ActivityVerbDto;
  routeId: string;
  routeName: string;
  detail: string;
  at: string;
};

export type ActivityPageDto = { items: ActivityItemDto[]; next_cursor: string | null };
