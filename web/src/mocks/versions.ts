import { DiffMark, type RouteVersion } from '../domain';

/** Design `VERS` — history of r1; the mock serves it for any route without its own history. */
export const VERSIONS: RouteVersion[] = [
  {
    routeId: 'r1',
    version: 4,
    author: 'Minh N.',
    createdAt: '2026-10-13T09:32:00Z',
    note: 'Mask email before sending to partner',
    diff: [
      { mark: DiffMark.Added, text: 'Mask email (email style)' },
      { mark: DiffMark.Removed, text: 'Remove email' },
      { mark: DiffMark.Changed, text: 'amount > 50  →  amount > 100' },
    ],
  },
  {
    routeId: 'r1',
    version: 3,
    author: 'Quincy N.',
    createdAt: '2026-10-10T14:05:00Z',
    note: 'Raise threshold, set key from orderId',
    diff: [
      { mark: DiffMark.Added, text: 'Set key from orderId' },
      { mark: DiffMark.Changed, text: 'Partitioning: round-robin → by key' },
    ],
  },
  {
    routeId: 'r1',
    version: 2,
    author: 'Quincy N.',
    createdAt: '2026-10-02T10:11:00Z',
    note: 'Rename cust_id for partner schema',
    diff: [{ mark: DiffMark.Added, text: 'Rename cust_id → customerId' }],
  },
  {
    routeId: 'r1',
    version: 1,
    author: 'Quincy N.',
    createdAt: '2026-09-28T17:40:00Z',
    note: 'Initial route',
    diff: [
      { mark: DiffMark.Added, text: 'Filter eventType = OrderPaid' },
      { mark: DiffMark.Added, text: 'Destination partner-kafka / orders-public' },
    ],
  },
];

export const VERSION_TEXT = {
  noNote: 'No change note',
  unchanged: 'Configuration unchanged',
} as const;
