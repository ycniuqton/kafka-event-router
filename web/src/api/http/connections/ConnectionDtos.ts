/** Connection health on the wire (mirror of Go `connections.HealthStatus`). */
export const HealthStatusDto = {
  Unknown: 'unknown',
  Healthy: 'healthy',
  Degraded: 'degraded',
  Unreachable: 'unreachable',
  AuthFailed: 'auth_failed',
} as const;
export type HealthStatusDto = (typeof HealthStatusDto)[keyof typeof HealthStatusDto];

/** Security modes on the wire (mirror of Go `kafka.SecurityMode`). */
export const SecurityModeDto = {
  Plaintext: 'plaintext',
  Ssl: 'ssl',
  Mtls: 'mtls',
  SaslPlain: 'sasl_plain',
  SaslScram256: 'sasl_scram_256',
  SaslScram512: 'sasl_scram_512',
  OAuthBearer: 'sasl_oauthbearer',
} as const;
export type SecurityModeDto = (typeof SecurityModeDto)[keyof typeof SecurityModeDto];

/** Checklist steps in run order (mirror of Go `connections.TestStep`). */
export const TestStepDto = {
  BrokersReachable: 'brokers_reachable',
  Authenticated: 'authenticated',
  ListTopics: 'list_topics',
  CanRead: 'can_read',
  CanWrite: 'can_write',
  SchemaRegistry: 'schema_registry',
} as const;
export type TestStepDto = (typeof TestStepDto)[keyof typeof TestStepDto];

/** NDJSON line kinds of the test stream. */
export const TestLineType = { Step: 'step', Result: 'result' } as const;
export type TestLineType = (typeof TestLineType)[keyof typeof TestLineType];

/** What the API says about a write-only secret. */
export const SecretStateDto = { Set: 'set', Missing: 'missing' } as const;
export type SecretStateDto = (typeof SecretStateDto)[keyof typeof SecretStateDto];

export type LastFailureDto = { step: string; reason: string; message: string; hint: string };

export type HealthDto = { status: HealthStatusDto; checkedAt: string | null; lastFailure: LastFailureDto | null };

export type RouteRefDto = { id: string; name: string };

export type TlsDto = { enabled: boolean; caPem?: string; serverName?: string; clientCertPem?: string };

/** Registry auth block on the wire. */
export type RegistryAuthDto = { mode: string; username?: string };

/** `schemaRegistry` of the resource: the secret is only `set` / `missing`. */
export type SchemaRegistryDto = { url: string; auth: RegistryAuthDto; secret?: SecretStateDto };

/** Writable `schemaRegistry`; `null` removes it. */
export type SchemaRegistryInputDto = { url: string; auth: RegistryAuthDto } | null;

/** Writable security block (OAuth fields only for `sasl_oauthbearer`). */
export type SecurityInputDto = { mode: SecurityModeDto; tls: TlsDto; username?: string; clientId?: string; tokenUrl?: string; scopes?: string[] };

/** `GET /connections/{id}` resource. */
export type ConnectionDto = {
  id: string;
  name: string;
  description: string;
  envTag: string;
  owner: { teamId: string; teamName?: string };
  bootstrapServers: string[];
  security: SecurityInputDto & { secretFields: Record<string, SecretStateDto> };
  schemaRegistry?: SchemaRegistryDto | null;
  properties: Record<string, string>;
  sensitive: boolean;
  allowTopicCreate: boolean;
  probeEnabled: boolean;
  kind: string;
  health: HealthDto;
  usedBy: { count: number; routes?: RouteRefDto[] };
  version: number;
  createdAt: string;
  updatedAt: string;
};

/** `POST /connections` body (also the shape PATCH merges into). */
export type ConnectionInputDto = {
  name: string;
  description: string;
  envTag: string;
  bootstrapServers: string[];
  security: SecurityInputDto;
  schemaRegistry: SchemaRegistryInputDto;
  properties?: Record<string, string>;
  sensitive: boolean;
  allowTopicCreate: boolean;
  probeEnabled: boolean;
  secrets?: { password?: string; clientKeyPem?: string; clientSecret?: string; schemaRegistrySecret?: string };
};

/** `PATCH /connections/{id}` merge patch: `null` property values remove the key. */
export type ConnectionPatchDto = Omit<ConnectionInputDto, 'properties'> & { properties: Record<string, string | null> };

/** `POST /connections/{id}/test` body. */
export type TestOptionsDto = { readTopic?: string; writeTopic?: string };

/** `POST /connections/test` body: a full draft plus test options and the edited connection's id. */
export type DraftTestDto = ConnectionInputDto & TestOptionsDto & { connectionId?: string };

/** `POST /connections/{id}/topics` body. */
export type TopicInputDto = { name: string; partitions?: number; replicationFactor?: number; retentionMs?: number };

/** Extension members of connection problems (`connection-in-use`, `cluster-unavailable`). */
export type ConnectionProblemExtras = { routes?: RouteRefDto[]; reason?: string; hint?: string };

/** One `{"type":"step"}` line. */
export type StepEventDto = {
  type: typeof TestLineType.Step;
  step: string;
  state: string;
  durationMs?: number;
  reason?: string;
  message?: string;
  hint?: string;
};

/** The final `{"type":"result"}` line. */
export type TestResultDto = {
  type: typeof TestLineType.Result;
  ok: boolean;
  health: HealthStatusDto;
  checkedAt: string;
  lastFailure?: LastFailureDto;
};

export type TestLineDto = StepEventDto | TestResultDto;

/** `GET /connections/{id}/topics` item. */
export type TopicDto = { name: string; partitions: number; replicationFactor: number; internal: boolean; ratePerSec: number | null };

/** `POST /connections/{id}/topics` response. */
export type CreatedTopicDto = { name: string; partitions: number; replicationFactor: number; retentionMs: number };

/** REST paths of connections (relative to `/api/v1`). */
export class ConnectionEndpoint {
  static readonly collection = '/connections';
  static readonly draftTest = '/connections/test';

  static item(id: string): string {
    return `/connections/${encodeURIComponent(id)}`;
  }

  static test(id: string): string {
    return `${ConnectionEndpoint.item(id)}/test`;
  }

  static topics(id: string): string {
    return `${ConnectionEndpoint.item(id)}/topics`;
  }
}

/** Query keys of `GET /connections/{id}/topics`. */
export const TopicQueryKey = { Text: 'q', IncludeInternal: 'includeInternal' } as const;

/** Problem types specific to connections (RFC 7807 `type`). */
export const ConnectionProblemType = {
  InUse: 'https://eventrouter.dev/problems/connection-in-use',
  TopicExists: 'https://eventrouter.dev/problems/topic-exists',
  TopicCreateDisabled: 'https://eventrouter.dev/problems/topic-create-disabled',
  ClusterUnavailable: 'https://eventrouter.dev/problems/cluster-unavailable',
} as const;
