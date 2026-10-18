import type { LibraryLookupTable, LibraryTransform, LookupRow } from '../domain';
import type { Step } from '../api/generated';

const updated = '2026-10-12T09:00:00Z';

/** Design `LIB`. */
export const LIBRARY: LibraryTransform[] = [
  { id: 'lib-1', name: 'mask-customer-pii', version: 'v3', latestVersion: 3, description: 'Mask email, hash cust_id, drop phone', usedBy: 4, team: 'Integrations', teamId: 'team-int', size: 3, rowVersion: 1, archived: false, updatedAt: updated },
  { id: 'lib-2', name: 'order-to-partner', version: 'v2', latestVersion: 2, description: 'Rename to partner schema, wrap in envelope', usedBy: 2, team: 'Payments', teamId: 'team-pay', size: 2, rowVersion: 1, archived: false, updatedAt: updated },
  { id: 'lib-3', name: 'normalize-currency', version: 'v1', latestVersion: 1, description: 'Convert amount to minor units, upper-case currency', usedBy: 3, team: 'Finance', teamId: 'team-fin', size: 1, rowVersion: 1, archived: false, updatedAt: updated },
];

/** Engine steps of every version of the design transforms (oldest first). */
export const LIBRARY_VERSIONS: Record<string, Step[][]> = {
  'lib-1': [
    [{ kind: 'mask', field: 'value.email', style: 'email' }],
    [{ kind: 'mask', field: 'value.email', style: 'email' }, { kind: 'hash', field: 'value.cust_id' }],
    [{ kind: 'mask', field: 'value.email', style: 'email' }, { kind: 'hash', field: 'value.cust_id' }, { kind: 'remove', fields: ['value.phone'] }],
  ],
  'lib-2': [[{ kind: 'rename', from: 'value.orderId', to: 'value.order_ref' }], [{ kind: 'rename', from: 'value.orderId', to: 'value.order_ref' }, { kind: 'move', from: 'value.amount', to: 'value.payment.amount' }]],
  'lib-3': [[{ kind: 'convert_type', field: 'value.amount', targetType: 'integer', onFailure: 'keep' }]],
};

/** Design lookup tables of the library page. */
export const LOOKUP_TABLES: LibraryLookupTable[] = [
  { id: 'lt-1', name: 'country-to-region', version: 'v4', latestVersion: 4, description: 'ISO country code → sales region', rows: 249, usedBy: 3, team: 'Finance', teamId: 'team-fin', size: 249, rowVersion: 1, archived: false, updatedAt: updated, keyType: 'string' },
  { id: 'lt-2', name: 'sku-to-category', version: 'v12', latestVersion: 12, description: 'Product SKU → catalog category and brand', rows: 18420, usedBy: 2, team: 'Logistics', teamId: 'team-log', size: 18420, rowVersion: 1, archived: false, updatedAt: updated, keyType: 'string' },
  { id: 'lt-3', name: 'partner-codes', version: 'v1', latestVersion: 1, description: 'Internal partner id → partner gateway code', rows: 36, usedBy: 1, team: 'Integrations', teamId: 'team-int', size: 36, rowVersion: 1, archived: false, updatedAt: updated, keyType: 'number' },
];

/** First rows of the latest version of each design table (the mock pages through these). */
export const LOOKUP_ROWS: Record<string, LookupRow[]> = {
  'lt-1': [
    { key: 'DE', value: 'EMEA' },
    { key: 'FR', value: 'EMEA' },
    { key: 'US', value: 'AMER' },
    { key: 'VN', value: 'APAC' },
    { key: 'JP', value: 'APAC' },
  ],
  'lt-2': [
    { key: 'SKU-1', value: { category: 'books', brand: 'Acme' } },
    { key: 'SKU-2', value: { category: 'toys', brand: 'Blox' } },
  ],
  'lt-3': [
    { key: '1', value: 'GW-ALPHA' },
    { key: '2', value: 'GW-BETA' },
  ],
};
