/** Lookup sources (connections 00001-lookup-sources): connection-like targets lookup steps call to enrich events. */
import { EnvTag } from './connection';
import { Tone } from './tone';

export const LookupSourceKind = { Http: 'http', Redis: 'redis', Postgres: 'postgres', Static: 'static' } as const;
export type LookupSourceKind = (typeof LookupSourceKind)[keyof typeof LookupSourceKind];

/** HTTP auth modes of a source. */
export const LookupAuthMode = { None: 'none', Bearer: 'bearer', Basic: 'basic', Header: 'header' } as const;
export type LookupAuthMode = (typeof LookupAuthMode)[keyof typeof LookupAuthMode];

/** Write-only secret members (never returned; `secretFields` lists the stored ones). */
export const LookupSecretField = { Token: 'token', Password: 'password', HeaderValue: 'headerValue' } as const;
export type LookupSecretField = (typeof LookupSecretField)[keyof typeof LookupSecretField];

export const LookupHealthStatus = { Unknown: 'unknown', Healthy: 'healthy', Unreachable: 'unreachable', AuthFailed: 'auth_failed', Degraded: 'degraded' } as const;
export type LookupHealthStatus = (typeof LookupHealthStatus)[keyof typeof LookupHealthStatus];

export const LookupTestStep = { Reachable: 'reachable', Authenticated: 'authenticated', Sample: 'sample' } as const;
export type LookupTestStep = (typeof LookupTestStep)[keyof typeof LookupTestStep];

export const LookupTestState = { Passed: 'passed', Failed: 'failed', Skipped: 'skipped' } as const;
export type LookupTestState = (typeof LookupTestState)[keyof typeof LookupTestState];

/** Postgres sslmode values offered by the form. */
export const LookupSslMode = { Disable: 'disable', Require: 'require', VerifyCa: 'verify-ca', VerifyFull: 'verify-full' } as const;
export type LookupSslMode = (typeof LookupSslMode)[keyof typeof LookupSslMode];

/** Request kind a lookup step uses for each source kind (pipeline-engine 00002 §1). */
export const LookupRequestKind = { Http: 'http', Redis: 'redis', Sql: 'sql', Static: 'static' } as const;
export type LookupRequestKind = (typeof LookupRequestKind)[keyof typeof LookupRequestKind];

export type LookupHttpSettings = {
  baseUrl: string;
  auth: { mode: LookupAuthMode; username: string; headerName: string };
  tls: { insecureSkipVerify: boolean; caPem: string };
  headers: Record<string, string>;
};
export type LookupRedisSettings = { addr: string; db: number; username: string; tls: boolean };
export type LookupPostgresSettings = { host: string; port: number; database: string; user: string; sslMode: LookupSslMode; statementTimeoutMs: number };
export type LookupStaticSettings = { tableRef: string; version: number };
export type LookupLimits = { maxConcurrency: number; ratePerSec: number; burst: number; breakerFailures: number; breakerOpenMs: number };
export type LookupSecrets = Partial<Record<LookupSecretField, string>>;
export type LookupSourceRouteRef = { id: string; name: string };
export type LookupFailure = { step: string; message: string; hint: string };

/** Settings of every kind; only the one of `kind` is sent and stored. */
export type LookupKindSettings = {
  http: LookupHttpSettings | null;
  redis: LookupRedisSettings | null;
  postgres: LookupPostgresSettings | null;
  static: LookupStaticSettings | null;
};

export type LookupSource = LookupKindSettings & {
  id: string;
  name: string;
  description: string;
  kind: LookupSourceKind;
  envTag: EnvTag;
  sensitive: boolean;
  owner: { teamId: string; teamName: string };
  limits: LookupLimits;
  secretFields: LookupSecretField[];
  health: { status: LookupHealthStatus; checkedAt: string | null; lastFailure: LookupFailure | null };
  usedBy: { count: number; routes: LookupSourceRouteRef[] };
  version: number;
  createdAt: string;
  updatedAt: string;
};

/** What the form edits: every kind's settings (switching kind keeps typed values) plus write-only secrets. */
export type LookupSourceDraft = {
  name: string;
  description: string;
  kind: LookupSourceKind;
  envTag: EnvTag;
  sensitive: boolean;
  ownerTeamId: string;
  http: LookupHttpSettings;
  redis: LookupRedisSettings;
  postgres: LookupPostgresSettings;
  static: LookupStaticSettings;
  limits: LookupLimits;
  secrets: LookupSecrets;
};

export type LookupTestStepResult = { step: LookupTestStep; state: LookupTestState; durationMs?: number; message?: string; hint?: string };
export type LookupTestResult = { ok: boolean; health: LookupHealthStatus; checkedAt: string; steps: LookupTestStepResult[] };

/** Deleting a source still used by routes. */
export class LookupSourceInUseError extends Error {
  readonly routes: LookupSourceRouteRef[];

  constructor(message: string, routes: LookupSourceRouteRef[]) {
    super(message);
    this.name = 'LookupSourceInUseError';
    this.routes = routes;
  }
}

/** Default settings of a new source (design §3 defaults). */
export class LookupSourceDefaults {
  static readonly limits: LookupLimits = { maxConcurrency: 16, ratePerSec: 0, burst: 0, breakerFailures: 5, breakerOpenMs: 10000 };
  static readonly postgresPort = 5432;
  static readonly statementTimeoutMs = 2000;

  static draft(kind: LookupSourceKind = LookupSourceKind.Http): LookupSourceDraft {
    return {
      name: '',
      description: '',
      kind,
      envTag: EnvTag.Dev,
      sensitive: false,
      ownerTeamId: '',
      http: { baseUrl: '', auth: { mode: LookupAuthMode.None, username: '', headerName: '' }, tls: { insecureSkipVerify: false, caPem: '' }, headers: {} },
      redis: { addr: '', db: 0, username: '', tls: false },
      postgres: { host: '', port: LookupSourceDefaults.postgresPort, database: '', user: '', sslMode: LookupSslMode.Require, statementTimeoutMs: LookupSourceDefaults.statementTimeoutMs },
      static: { tableRef: '', version: 1 },
      limits: { ...LookupSourceDefaults.limits },
      secrets: {},
    };
  }

  /** The edit draft of a saved source (secrets blank = keep the stored ones). */
  static fromSource(s: LookupSource): LookupSourceDraft {
    const base = LookupSourceDefaults.draft(s.kind);
    return {
      ...base,
      name: s.name,
      description: s.description,
      envTag: s.envTag,
      sensitive: s.sensitive,
      ownerTeamId: s.owner.teamId,
      http: s.http ?? base.http,
      redis: s.redis ?? base.redis,
      postgres: s.postgres ?? base.postgres,
      static: s.static ?? base.static,
      limits: { ...s.limits },
    };
  }
}

/** Labels, request kind and the "target" column of each source kind. */
export const LookupSourceKindMeta: Record<LookupSourceKind, { label: string; requestKind: LookupRequestKind; target: (s: LookupKindSettings) => string }> = {
  [LookupSourceKind.Http]: { label: 'HTTP', requestKind: LookupRequestKind.Http, target: (s) => s.http?.baseUrl ?? '' },
  [LookupSourceKind.Redis]: { label: 'Redis', requestKind: LookupRequestKind.Redis, target: (s) => (s.redis ? `${s.redis.addr} / db ${s.redis.db}` : '') },
  [LookupSourceKind.Postgres]: {
    label: 'Postgres',
    requestKind: LookupRequestKind.Sql,
    target: (s) => (s.postgres ? `${s.postgres.host}:${s.postgres.port}/${s.postgres.database}` : ''),
  },
  [LookupSourceKind.Static]: { label: 'Static table', requestKind: LookupRequestKind.Static, target: (s) => (s.static ? `${s.static.tableRef} v${s.static.version}` : '') },
};

export const LookupAuthModeLabel: Record<LookupAuthMode, string> = {
  [LookupAuthMode.None]: 'None',
  [LookupAuthMode.Bearer]: 'Bearer token',
  [LookupAuthMode.Basic]: 'Basic',
  [LookupAuthMode.Header]: 'Custom header',
};

export const LookupSecretLabel: Record<LookupSecretField, string> = {
  [LookupSecretField.Token]: 'Token',
  [LookupSecretField.Password]: 'Password',
  [LookupSecretField.HeaderValue]: 'Header value',
};

export const LookupHealthMeta: Record<LookupHealthStatus, { label: string; tone: Tone }> = {
  [LookupHealthStatus.Healthy]: { label: 'Healthy', tone: Tone.Ok },
  [LookupHealthStatus.AuthFailed]: { label: 'Auth failed', tone: Tone.Err },
  [LookupHealthStatus.Unreachable]: { label: 'Unreachable', tone: Tone.Err },
  [LookupHealthStatus.Degraded]: { label: 'Degraded', tone: Tone.Warn },
  [LookupHealthStatus.Unknown]: { label: 'Not tested', tone: Tone.Off },
};

export const LookupTestStepLabel: Record<LookupTestStep, string> = {
  [LookupTestStep.Reachable]: 'Source reachable',
  [LookupTestStep.Authenticated]: 'Authenticated',
  [LookupTestStep.Sample]: 'Sample request answered',
};

/** Validation messages of the source form (keyed by JSON Pointer, as the API reports them). */
export const LookupSourceMessage = {
  Name: 'Enter a name.',
  BaseUrl: 'Enter an http:// or https:// base URL.',
  Username: 'Enter the username.',
  HeaderName: 'Enter the header name.',
  Secret: 'Enter the secret.',
  Addr: 'Enter host:port.',
  Db: 'Use a database number from 0 to 15.',
  Host: 'Enter the host.',
  Port: 'Use a port from 1 to 65535.',
  Database: 'Enter the database name.',
  User: 'Enter the user.',
  Timeout: 'Use a timeout from 1 to 60000 ms.',
  Table: 'Pick a library lookup table.',
  Version: 'Pick a table version.',
  Positive: 'Use a number of at least 1.',
  NonNegative: 'Use 0 or more.',
} as const;

export type LookupFieldErrors = Record<string, string>;

/** Numeric bounds of the form fields. */
export const LookupBounds = { maxDb: 15, maxPort: 65535, maxTimeoutMs: 60000 } as const;

/** Which secrets a draft needs and whether it is complete enough to save or test. */
export class LookupSourceRules {
  static readonly urlPattern = /^https?:\/\/\S+$/;
  static readonly addrPattern = /^[^\s:]+:\d+$/;

  /** Secret members the draft's kind / auth mode uses. */
  static secretFields(d: Pick<LookupSourceDraft, 'kind' | 'http' | 'redis'>): LookupSecretField[] {
    if (d.kind === LookupSourceKind.Postgres) return [LookupSecretField.Password];
    if (d.kind === LookupSourceKind.Redis) return d.redis.username ? [LookupSecretField.Password] : [];
    if (d.kind !== LookupSourceKind.Http) return [];
    return LookupSourceRules.httpSecrets[d.http.auth.mode];
  }

  private static readonly httpSecrets: Record<LookupAuthMode, LookupSecretField[]> = {
    [LookupAuthMode.None]: [],
    [LookupAuthMode.Bearer]: [LookupSecretField.Token],
    [LookupAuthMode.Basic]: [LookupSecretField.Password],
    [LookupAuthMode.Header]: [LookupSecretField.HeaderValue],
  };

  /** Field errors by JSON Pointer; `stored` secrets count as present (edit keeps them). */
  static validate(d: LookupSourceDraft, stored: LookupSecretField[] = []): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!d.name.trim()) errors['/name'] = LookupSourceMessage.Name;
    Object.assign(errors, LookupKindValidators.of(d.kind)(d), LookupSourceRules.limitErrors(d.limits));
    for (const f of LookupSourceRules.secretFields(d)) {
      if (!d.secrets[f]?.trim() && !stored.includes(f)) errors[`/secrets/${f}`] = LookupSourceMessage.Secret;
    }
    return errors;
  }

  private static limitErrors(l: LookupLimits): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!(l.maxConcurrency >= 1)) errors['/limits/maxConcurrency'] = LookupSourceMessage.Positive;
    if (!(l.breakerFailures >= 1)) errors['/limits/breakerFailures'] = LookupSourceMessage.Positive;
    if (!(l.breakerOpenMs >= 1)) errors['/limits/breakerOpenMs'] = LookupSourceMessage.Positive;
    if (!(l.ratePerSec >= 0)) errors['/limits/ratePerSec'] = LookupSourceMessage.NonNegative;
    if (!(l.burst >= 0)) errors['/limits/burst'] = LookupSourceMessage.NonNegative;
    return errors;
  }
}

/** Kind-specific validators (registry: one entry per source kind). */
export class LookupKindValidators {
  private static readonly validators: Record<LookupSourceKind, (d: LookupSourceDraft) => LookupFieldErrors> = {
    [LookupSourceKind.Http]: (d) => LookupKindValidators.http(d.http),
    [LookupSourceKind.Redis]: (d) => LookupKindValidators.redis(d.redis),
    [LookupSourceKind.Postgres]: (d) => LookupKindValidators.postgres(d.postgres),
    [LookupSourceKind.Static]: (d) => LookupKindValidators.staticTable(d.static),
  };

  static of(kind: LookupSourceKind): (d: LookupSourceDraft) => LookupFieldErrors {
    return LookupKindValidators.validators[kind];
  }

  private static http(h: LookupHttpSettings): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!LookupSourceRules.urlPattern.test(h.baseUrl.trim())) errors['/http/baseUrl'] = LookupSourceMessage.BaseUrl;
    if (h.auth.mode === LookupAuthMode.Basic && !h.auth.username.trim()) errors['/http/auth/username'] = LookupSourceMessage.Username;
    if (h.auth.mode === LookupAuthMode.Header && !h.auth.headerName.trim()) errors['/http/auth/headerName'] = LookupSourceMessage.HeaderName;
    return errors;
  }

  private static redis(r: LookupRedisSettings): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!LookupSourceRules.addrPattern.test(r.addr.trim())) errors['/redis/addr'] = LookupSourceMessage.Addr;
    if (!Number.isInteger(r.db) || r.db < 0 || r.db > LookupBounds.maxDb) errors['/redis/db'] = LookupSourceMessage.Db;
    return errors;
  }

  private static postgres(p: LookupPostgresSettings): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!p.host.trim()) errors['/postgres/host'] = LookupSourceMessage.Host;
    if (!Number.isInteger(p.port) || p.port < 1 || p.port > LookupBounds.maxPort) errors['/postgres/port'] = LookupSourceMessage.Port;
    if (!p.database.trim()) errors['/postgres/database'] = LookupSourceMessage.Database;
    if (!p.user.trim()) errors['/postgres/user'] = LookupSourceMessage.User;
    if (!(p.statementTimeoutMs >= 1 && p.statementTimeoutMs <= LookupBounds.maxTimeoutMs)) errors['/postgres/statementTimeoutMs'] = LookupSourceMessage.Timeout;
    return errors;
  }

  private static staticTable(s: LookupStaticSettings): LookupFieldErrors {
    const errors: LookupFieldErrors = {};
    if (!s.tableRef.trim()) errors['/static/tableRef'] = LookupSourceMessage.Table;
    if (!(s.version >= 1)) errors['/static/version'] = LookupSourceMessage.Version;
    return errors;
  }
}

/** The settings of a draft's own kind only (others null), as stored and sent. */
export class LookupKindSettingsOf {
  static readonly none: LookupKindSettings = { http: null, redis: null, postgres: null, static: null };

  static draft(d: LookupSourceDraft): LookupKindSettings {
    return { ...LookupKindSettingsOf.none, [d.kind]: d[d.kind] };
  }
}
