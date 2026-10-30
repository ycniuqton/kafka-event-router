import { EnvTag, LookupAuthMode, LookupHealthStatus, LookupSecretField, LookupSourceDefaults, LookupSourceKind, type LookupSource } from '../domain';
import { MockTeamId } from './members';

const seeded = '2026-10-10T08:00:00Z';
const checked = '2026-10-13T09:30:00Z';
const unused = { count: 0, routes: [] };

/** Lookup sources of the Connections → Lookup sources tab (HTTP CRM, Redis flags, static price list). */
export const LOOKUP_SOURCES: LookupSource[] = [
  {
    id: 'ls-crm-api', name: 'crm-api', description: 'Customer tier and segment by customer id', kind: LookupSourceKind.Http, envTag: EnvTag.Prod, sensitive: true,
    owner: { teamId: MockTeamId.Payments, teamName: 'Payments' },
    http: { baseUrl: 'https://crm.internal/api/v2', auth: { mode: LookupAuthMode.Bearer, username: '', headerName: '' }, tls: { insecureSkipVerify: false, caPem: '' }, headers: { Accept: 'application/json' } },
    redis: null, postgres: null, static: null,
    limits: { ...LookupSourceDefaults.limits, ratePerSec: 200, burst: 50 },
    secretFields: [LookupSecretField.Token],
    health: { status: LookupHealthStatus.Healthy, checkedAt: checked, lastFailure: null },
    usedBy: unused, version: 3, createdAt: seeded, updatedAt: checked,
  },
  {
    id: 'ls-flags', name: 'feature-flags', description: 'Per-merchant feature flags (hash per merchant)', kind: LookupSourceKind.Redis, envTag: EnvTag.Staging, sensitive: false,
    owner: { teamId: MockTeamId.Platform, teamName: 'Platform' },
    http: null, redis: { addr: 'redis-flags.internal:6379', db: 2, username: '', tls: false }, postgres: null, static: null,
    limits: { ...LookupSourceDefaults.limits },
    secretFields: [],
    health: { status: LookupHealthStatus.Unknown, checkedAt: null, lastFailure: null },
    usedBy: unused, version: 1, createdAt: seeded, updatedAt: seeded,
  },
  {
    id: 'ls-price-list', name: 'price-list', description: 'Country → sales region (library table)', kind: LookupSourceKind.Static, envTag: EnvTag.Dev, sensitive: false,
    owner: { teamId: MockTeamId.Logistics, teamName: 'Logistics' },
    http: null, redis: null, postgres: null, static: { tableRef: 'country-to-region', version: 4 },
    limits: { ...LookupSourceDefaults.limits },
    secretFields: [],
    health: { status: LookupHealthStatus.Healthy, checkedAt: checked, lastFailure: null },
    usedBy: unused, version: 2, createdAt: seeded, updatedAt: checked,
  },
];
