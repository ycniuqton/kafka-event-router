import { Tone } from './tone';

/** Health of a saved Kafka connection after its last test. */
export const ConnectionStatus = {
  Healthy: 'healthy',
  AuthFailed: 'auth',
  Unreachable: 'unreach',
  Degraded: 'degraded',
  Untested: 'untested',
} as const;
export type ConnectionStatus = (typeof ConnectionStatus)[keyof typeof ConnectionStatus];

/** Environment a cluster belongs to. */
export const EnvTag = { Prod: 'prod', Staging: 'staging', Dev: 'dev' } as const;

/** Cluster behind a connection: a real Kafka cluster or the in-memory demo cluster ("Try with demo data"). */
export const ClusterKind = { Kafka: 'kafka', Demo: 'demo' } as const;
export type ClusterKind = (typeof ClusterKind)[keyof typeof ClusterKind];
export type EnvTag = (typeof EnvTag)[keyof typeof EnvTag];

/** Security modes offered by the create-connection form (existing clusters may show variants). */
export const SecurityMode = {
  None: 'None',
  Tls: 'TLS',
  SaslPlain: 'SASL/PLAIN',
  SaslScram: 'SASL/SCRAM',
  Mtls: 'mTLS',
  OAuth: 'OAuth',
} as const;
export type SecurityMode = (typeof SecurityMode)[keyof typeof SecurityMode];

/** SCRAM hash variants offered when SASL/SCRAM is selected. */
export const ScramMechanism = { Sha512: 'SCRAM-SHA-512', Sha256: 'SCRAM-SHA-256' } as const;
export type ScramMechanism = (typeof ScramMechanism)[keyof typeof ScramMechanism];

/** Security modes offered by the panel (OAuth = SASL/OAUTHBEARER with client credentials, CN-109). */
export const OfferedSecurityModes: readonly SecurityMode[] = [
  SecurityMode.None,
  SecurityMode.Tls,
  SecurityMode.SaslPlain,
  SecurityMode.SaslScram,
  SecurityMode.Mtls,
  SecurityMode.OAuth,
];

/** Write-only secret fields of a connection (values are never returned, only whether one is stored). */
export const SecretField = {
  Password: 'password',
  ClientKeyPem: 'clientKeyPem',
  ClientSecret: 'clientSecret',
  SchemaRegistrySecret: 'schemaRegistrySecret',
} as const;
export type SecretField = (typeof SecretField)[keyof typeof SecretField];

/** Whether a security mode runs over TLS: never, always, or when the user ticks "over TLS". */
export const TlsUsage = { Off: 'off', Required: 'required', Optional: 'optional' } as const;
export type TlsUsage = (typeof TlsUsage)[keyof typeof TlsUsage];

/** Per-mode facts shared by the panel and the API mapping: TLS usage and the write-only secrets it needs. */
export class SecurityModeRules {
  static readonly tls: Record<SecurityMode, TlsUsage> = {
    [SecurityMode.None]: TlsUsage.Off,
    [SecurityMode.Tls]: TlsUsage.Required,
    [SecurityMode.SaslPlain]: TlsUsage.Optional,
    [SecurityMode.SaslScram]: TlsUsage.Optional,
    [SecurityMode.Mtls]: TlsUsage.Required,
    [SecurityMode.OAuth]: TlsUsage.Optional,
  };

  static readonly secrets: Record<SecurityMode, readonly SecretField[]> = {
    [SecurityMode.None]: [],
    [SecurityMode.Tls]: [],
    [SecurityMode.SaslPlain]: [SecretField.Password],
    [SecurityMode.SaslScram]: [SecretField.Password],
    [SecurityMode.Mtls]: [SecretField.ClientKeyPem],
    [SecurityMode.OAuth]: [SecretField.ClientSecret],
  };

  static usesUsername(mode: SecurityMode): boolean {
    return SecurityModeRules.secrets[mode].includes(SecretField.Password);
  }

  static usesClientCert(mode: SecurityMode): boolean {
    return SecurityModeRules.secrets[mode].includes(SecretField.ClientKeyPem);
  }

  static usesOAuth(mode: SecurityMode): boolean {
    return SecurityModeRules.secrets[mode].includes(SecretField.ClientSecret);
  }

  /** Effective TLS flag for a mode given the "over TLS" choice. */
  static tlsEnabled(mode: SecurityMode, requested: boolean): boolean {
    const usage = SecurityModeRules.tls[mode];
    return usage === TlsUsage.Required || (usage === TlsUsage.Optional && requested);
  }
}

/** How the router authenticates against a schema registry. */
export const RegistryAuthMode = { None: 'none', Basic: 'basic', Bearer: 'bearer' } as const;
export type RegistryAuthMode = (typeof RegistryAuthMode)[keyof typeof RegistryAuthMode];

/** Per-mode registry facts: whether a username and a write-only secret (password or token) are needed. */
export class RegistryAuthRules {
  static readonly labels: Record<RegistryAuthMode, string> = {
    [RegistryAuthMode.None]: 'None',
    [RegistryAuthMode.Basic]: 'Basic',
    [RegistryAuthMode.Bearer]: 'Bearer token',
  };

  static usesUsername(mode: RegistryAuthMode): boolean {
    return mode === RegistryAuthMode.Basic;
  }

  static usesSecret(mode: RegistryAuthMode): boolean {
    return mode !== RegistryAuthMode.None;
  }
}

/** Saved schema registry of a connection (the password or token is only known as stored or not). */
export type ConnectionSchemaRegistry = { url: string; authMode: RegistryAuthMode; username: string; secretStored: boolean };

/** A topic on a cluster; `rate` is a display label such as "1.2k/s" or "—". */
export type TopicInfo = { name: string; partitions: number; rate: string; replicationFactor?: number; internal?: boolean };

/** Non-secret TLS settings (PEM text; empty strings when unset). */
export type ConnectionTls = { enabled: boolean; caPem: string; serverName: string; clientCertPem: string };

/** Saved security settings of a connection, as needed to pre-fill the edit panel. */
export type ConnectionSecurity = {
  mode: SecurityMode;
  scram: ScramMechanism;
  tls: ConnectionTls;
  username: string;
  storedSecrets: SecretField[];
  clientId?: string;
  tokenUrl?: string;
  scopes?: string[];
};

/** Why the last test or probe failed (plain-language message + hint). */
export type ConnectionFailure = { step: string; reason: string; message: string; hint: string };

/** A route referencing a connection. */
export type ConnectionRouteRef = { id: string; name: string };

/** Routes using a connection: the list returns `count` only, GET-one also the routes. */
export type ConnectionUsage = { count: number; routes: ConnectionRouteRef[] | null };

/** A saved Kafka cluster (design `CONNS`). Mock data uses the name as `id`; the REST API sets `name` and the details. */
export type Connection = {
  id: string;
  name?: string;
  env: EnvTag;
  bootstrapServers: string[];
  security: string;
  status: ConnectionStatus;
  checkedAt: string | null;
  owner: string;
  topics: TopicInfo[];
  description?: string;
  allowTopicCreate?: boolean;
  probeEnabled?: boolean;
  sensitive?: boolean;
  kind?: string;
  version?: number;
  lastFailure?: ConnectionFailure | null;
  usedBy?: ConnectionUsage;
  securityDetail?: ConnectionSecurity;
  schemaRegistry?: ConnectionSchemaRegistry | null;
  properties?: Record<string, string>;
};

/** Input of the 4-step "New connection" panel; empty secrets mean "keep the stored one" when editing. */
export type ConnectionDraft = {
  name: string;
  description: string;
  env: EnvTag;
  team: string;
  bootstrapServers: string[];
  security: SecurityMode;
  username: string;
  password: string;
  /** Empty = no schema registry. */
  schemaRegistryUrl: string;
  schemaRegistryAuth?: RegistryAuthMode;
  schemaRegistryUsername?: string;
  /** Registry password (basic) or token (bearer); empty keeps the stored one. */
  schemaRegistrySecret?: string;
  clientId?: string;
  tokenUrl?: string;
  scopes?: string[];
  clientSecret?: string;
  sensitive?: boolean;
  scram?: ScramMechanism;
  tls?: ConnectionTls;
  clientKeyPem?: string;
  properties?: Record<string, string>;
  allowTopicCreate?: boolean;
  probeEnabled?: boolean;
};

/** Optional topics used by the "can read" / "can write" checks. */
export type ConnectionTestOptions = { readTopic?: string; writeTopic?: string };

/** Which topics the browser lists. */
export type TopicQuery = { includeInternal?: boolean; q?: string };

/** Settings of a new topic; omitted values take the cluster / product defaults. */
export type TopicSettings = { partitions?: number; replicationFactor?: number; retentionMs?: number };

/** Product defaults and limits for new topics (spec §5.4: 6 partitions, 7 d retention). */
export class TopicDefaults {
  static readonly partitions = 6;
  static readonly retentionDays = 7;
  static readonly msPerDay = 86_400_000;
  static readonly maxPartitions = 1000;
  static readonly maxNameLength = 249;
  static readonly namePattern = /^[a-zA-Z0-9._-]+$/;

  static retentionMs(days: number): number {
    return days * TopicDefaults.msPerDay;
  }
}

/** Raised by `remove` when routes still use the connection. */
export class ConnectionInUseError extends Error {
  readonly routes: ConnectionRouteRef[];

  constructor(message: string, routes: ConnectionRouteRef[]) {
    super(message);
    this.name = 'ConnectionInUseError';
    this.routes = routes;
  }
}

export const ConnectionStatusMeta: Record<ConnectionStatus, { label: string; tone: Tone }> = {
  [ConnectionStatus.Healthy]: { label: 'Healthy', tone: Tone.Ok },
  [ConnectionStatus.AuthFailed]: { label: 'Auth failed', tone: Tone.Err },
  [ConnectionStatus.Unreachable]: { label: 'Unreachable', tone: Tone.Err },
  [ConnectionStatus.Degraded]: { label: 'Degraded', tone: Tone.Warn },
  [ConnectionStatus.Untested]: { label: 'Not tested', tone: Tone.Off },
};

/** Env tag colours: prod magenta, staging blue, dev grey. */
export const EnvMeta: Record<EnvTag, { label: string; tone: Tone }> = {
  [EnvTag.Prod]: { label: 'prod', tone: Tone.Prod },
  [EnvTag.Staging]: { label: 'staging', tone: Tone.Info },
  [EnvTag.Dev]: { label: 'dev', tone: Tone.Muted },
};

/** State of one check in the connection test checklist. */
export const TestStepState = {
  Pending: 'pending',
  Running: 'running',
  Passed: 'passed',
  Failed: 'failed',
  Skipped: 'skipped',
} as const;
export type TestStepState = (typeof TestStepState)[keyof typeof TestStepState];

/** The five checks run by "Test connection", in order. */
export const ConnectionTestChecks = [
  'Brokers reachable',
  'Authenticated',
  'Can list topics',
  'Can read (sample topic)',
  'Can write (optional)',
] as const;

/** Checks that only run for some connections, by server step id (shown once the server reports them). */
export const OptionalTestChecks: Record<string, string> = { schema_registry: 'Schema registry reachable' };

/** One checklist row; `reason` is message + hint joined, kept for compact views. */
export type ConnectionTestStep = { label: string; state: TestStepState; reason?: string; message?: string; hint?: string; durationMs?: number };

/** Progress snapshot of a running or finished connection test. */
export type ConnectionTestRun = {
  connectionId: string;
  steps: ConnectionTestStep[];
  finished: boolean;
  ok: boolean;
  /** Set when the run could not start or broke off (validation, network, stream cut). */
  error?: string;
};

/** Key of the unsaved-draft test run in the store (`connectionTests`). */
export const DraftTestKey = { create: 'draft:new', editPrefix: 'draft:' } as const;

/** Display helpers for connections. */
export class ConnectionDisplay {
  /** The connection's name (mock connections are keyed by name). */
  static name(connection: Connection): string {
    return connection.name ?? connection.id;
  }

  /** Demo connections run on the in-memory demo cluster; their brokers and security are fixed. */
  static isDemo(connection: Connection): boolean {
    return connection.kind === ClusterKind.Demo;
  }

  /** Whether the edit panel applies: demo connections keep their fixed cluster settings. */
  static editable(connection: Connection): boolean {
    return !ConnectionDisplay.isDemo(connection);
  }

  /** Store key of a panel test: the edited connection's draft or the new-connection draft. */
  static draftTestKey(connectionId: string | null): string {
    return connectionId ? DraftTestKey.editPrefix + connectionId : DraftTestKey.create;
  }
}
