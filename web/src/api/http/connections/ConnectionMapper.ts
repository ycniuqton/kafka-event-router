import {
  ConnectionStatus,
  EnvTag,
  RegistryAuthMode,
  RegistryAuthRules,
  ScramMechanism,
  SecretField,
  SecurityMode,
  SecurityModeRules,
  type Connection,
  type ConnectionDraft,
  type ConnectionSchemaRegistry,
  type ConnectionSecurity,
  type ConnectionTls,
  type TopicInfo,
} from '../../../domain';
import {
  HealthStatusDto,
  SecretStateDto,
  SecurityModeDto,
  type ConnectionDto,
  type ConnectionInputDto,
  type ConnectionPatchDto,
  type CreatedTopicDto,
  type SchemaRegistryDto,
  type SchemaRegistryInputDto,
  type SecurityInputDto,
  type TlsDto,
  type TopicDto,
} from './ConnectionDtos';

/** Maps backend health onto the screen statuses (never-tested = "Not tested"). */
export const HealthStatusMap: Record<HealthStatusDto, ConnectionStatus> = {
  [HealthStatusDto.Unknown]: ConnectionStatus.Untested,
  [HealthStatusDto.Healthy]: ConnectionStatus.Healthy,
  [HealthStatusDto.Degraded]: ConnectionStatus.Degraded,
  [HealthStatusDto.Unreachable]: ConnectionStatus.Unreachable,
  [HealthStatusDto.AuthFailed]: ConnectionStatus.AuthFailed,
};

/** Security labels shown in the table. */
export const SecurityModeLabel: Record<SecurityModeDto, string> = {
  [SecurityModeDto.Plaintext]: SecurityMode.None,
  [SecurityModeDto.Ssl]: SecurityMode.Tls,
  [SecurityModeDto.Mtls]: SecurityMode.Mtls,
  [SecurityModeDto.SaslPlain]: SecurityMode.SaslPlain,
  [SecurityModeDto.SaslScram256]: `${SecurityMode.SaslScram}-256`,
  [SecurityModeDto.SaslScram512]: `${SecurityMode.SaslScram}-512`,
  [SecurityModeDto.OAuthBearer]: SecurityMode.OAuth,
};

/** Wire mode → panel mode + SCRAM variant. */
export const WireSecurityMode: Record<SecurityModeDto, { mode: SecurityMode; scram: ScramMechanism }> = {
  [SecurityModeDto.Plaintext]: { mode: SecurityMode.None, scram: ScramMechanism.Sha512 },
  [SecurityModeDto.Ssl]: { mode: SecurityMode.Tls, scram: ScramMechanism.Sha512 },
  [SecurityModeDto.Mtls]: { mode: SecurityMode.Mtls, scram: ScramMechanism.Sha512 },
  [SecurityModeDto.SaslPlain]: { mode: SecurityMode.SaslPlain, scram: ScramMechanism.Sha512 },
  [SecurityModeDto.SaslScram256]: { mode: SecurityMode.SaslScram, scram: ScramMechanism.Sha256 },
  [SecurityModeDto.SaslScram512]: { mode: SecurityMode.SaslScram, scram: ScramMechanism.Sha512 },
  [SecurityModeDto.OAuthBearer]: { mode: SecurityMode.OAuth, scram: ScramMechanism.Sha512 },
};

/** SCRAM variant → wire mode. */
export const ScramWireMode: Record<ScramMechanism, SecurityModeDto> = {
  [ScramMechanism.Sha512]: SecurityModeDto.SaslScram512,
  [ScramMechanism.Sha256]: SecurityModeDto.SaslScram256,
};

/** Panel modes → wire modes (SCRAM picks its variant). */
export const DraftSecurityMode: Record<SecurityMode, (scram: ScramMechanism) => SecurityModeDto> = {
  [SecurityMode.None]: () => SecurityModeDto.Plaintext,
  [SecurityMode.Tls]: () => SecurityModeDto.Ssl,
  [SecurityMode.SaslPlain]: () => SecurityModeDto.SaslPlain,
  [SecurityMode.SaslScram]: (scram) => ScramWireMode[scram],
  [SecurityMode.Mtls]: () => SecurityModeDto.Mtls,
  [SecurityMode.OAuth]: () => SecurityModeDto.OAuthBearer,
};

/** Compact rate labels ("—" until the server can measure a rate). */
export class RateLabel {
  static readonly placeholder = '—';
  static readonly thousand = 1000;
  static readonly smallRate = 10;

  static of(ratePerSec: number | null): string {
    if (ratePerSec === null) return RateLabel.placeholder;
    if (ratePerSec >= RateLabel.thousand) return `${(ratePerSec / RateLabel.thousand).toFixed(1)}k/s`;
    return `${ratePerSec < RateLabel.smallRate ? ratePerSec.toFixed(1) : Math.round(ratePerSec)}/s`;
  }
}

/** The optional schema registry of a draft (an empty URL means none). */
class DraftRegistry {
  static mode(draft: ConnectionDraft): RegistryAuthMode {
    return draft.schemaRegistryAuth ?? RegistryAuthMode.None;
  }

  static input(draft: ConnectionDraft): SchemaRegistryInputDto {
    const url = draft.schemaRegistryUrl.trim();
    if (!url) return null;
    const mode = DraftRegistry.mode(draft);
    return { url, auth: { mode, username: RegistryAuthRules.usesUsername(mode) ? (draft.schemaRegistryUsername ?? '') : '' } };
  }

  static secretFields(draft: ConnectionDraft): SecretField[] {
    return DraftRegistry.input(draft) && RegistryAuthRules.usesSecret(DraftRegistry.mode(draft)) ? [SecretField.SchemaRegistrySecret] : [];
  }
}

/** Secret values of a draft by field; blanks are left out (= keep the stored secret). */
class DraftSecrets {
  static of(draft: ConnectionDraft): ConnectionInputDto['secrets'] {
    const values: Record<SecretField, string> = {
      [SecretField.Password]: draft.password,
      [SecretField.ClientKeyPem]: draft.clientKeyPem ?? '',
      [SecretField.ClientSecret]: draft.clientSecret ?? '',
      [SecretField.SchemaRegistrySecret]: draft.schemaRegistrySecret ?? '',
    };
    const used = [...SecurityModeRules.secrets[draft.security], ...DraftRegistry.secretFields(draft)];
    const entries = used.filter((f) => values[f]).map((f) => [f, values[f]]);
    return entries.length ? Object.fromEntries(entries) : undefined;
  }
}

/** Converts connection DTOs to the screen domain and drafts to request bodies. */
export class ConnectionMapper {
  static readonly noTls: ConnectionTls = { enabled: false, caPem: '', serverName: '', clientCertPem: '' };

  static connection(dto: ConnectionDto): Connection {
    return {
      id: dto.id,
      name: dto.name,
      env: ConnectionMapper.env(dto.envTag),
      bootstrapServers: dto.bootstrapServers,
      security: SecurityModeLabel[dto.security.mode] ?? dto.security.mode,
      status: HealthStatusMap[dto.health.status] ?? ConnectionStatus.Untested,
      checkedAt: dto.health.checkedAt,
      owner: dto.owner.teamName ?? dto.owner.teamId,
      topics: [],
      description: dto.description,
      allowTopicCreate: dto.allowTopicCreate,
      probeEnabled: dto.probeEnabled,
      sensitive: dto.sensitive,
      kind: dto.kind,
      version: dto.version,
      lastFailure: dto.health.lastFailure,
      usedBy: { count: dto.usedBy.count, routes: dto.usedBy.routes ?? null },
      securityDetail: ConnectionMapper.security(dto),
      schemaRegistry: ConnectionMapper.registry(dto.schemaRegistry),
      properties: dto.properties ?? {},
    };
  }

  static input(draft: ConnectionDraft): ConnectionInputDto {
    const mode = DraftSecurityMode[draft.security](draft.scram ?? ScramMechanism.Sha512);
    return {
      name: draft.name,
      description: draft.description,
      envTag: draft.env,
      bootstrapServers: draft.bootstrapServers,
      security: ConnectionMapper.securityInput(draft, mode),
      schemaRegistry: DraftRegistry.input(draft),
      properties: draft.properties ?? {},
      sensitive: draft.sensitive ?? false,
      allowTopicCreate: draft.allowTopicCreate ?? false,
      probeEnabled: draft.probeEnabled ?? true,
      secrets: DraftSecrets.of(draft),
    };
  }

  /** Merge patch from a draft; properties missing from the draft are removed (`null`). */
  static patch(draft: ConnectionDraft, previous: Record<string, string>): ConnectionPatchDto {
    const body = ConnectionMapper.input(draft);
    const removed = Object.keys(previous).filter((k) => !(k in (body.properties ?? {})));
    return { ...body, properties: { ...Object.fromEntries(removed.map((k) => [k, null])), ...body.properties } };
  }

  static topic(dto: TopicDto): TopicInfo {
    return { name: dto.name, partitions: dto.partitions, rate: RateLabel.of(dto.ratePerSec), replicationFactor: dto.replicationFactor, internal: dto.internal };
  }

  static createdTopic(dto: CreatedTopicDto): TopicInfo {
    return { name: dto.name, partitions: dto.partitions, rate: RateLabel.placeholder, replicationFactor: dto.replicationFactor };
  }

  /** OAuth fields are always sent (empty unless OAuth) so a merge patch clears them after a mode change. */
  private static securityInput(draft: ConnectionDraft, mode: SecurityModeDto): SecurityInputDto {
    const oauth = SecurityModeRules.usesOAuth(draft.security);
    return {
      mode,
      tls: ConnectionMapper.tls(draft),
      username: SecurityModeRules.usesUsername(draft.security) ? draft.username : '',
      clientId: oauth ? (draft.clientId ?? '') : '',
      tokenUrl: oauth ? (draft.tokenUrl ?? '') : '',
      scopes: oauth ? (draft.scopes ?? []) : [],
    };
  }

  private static registry(dto: SchemaRegistryDto | null | undefined): ConnectionSchemaRegistry | null {
    if (!dto) return null;
    const known = (Object.values(RegistryAuthMode) as string[]).includes(dto.auth.mode);
    const authMode = known ? (dto.auth.mode as RegistryAuthMode) : RegistryAuthMode.None;
    return { url: dto.url, authMode, username: dto.auth.username ?? '', secretStored: dto.secret === SecretStateDto.Set };
  }

  private static tls(draft: ConnectionDraft): TlsDto {
    const tls = draft.tls ?? ConnectionMapper.noTls;
    const enabled = SecurityModeRules.tlsEnabled(draft.security, tls.enabled);
    const clientCert = SecurityModeRules.usesClientCert(draft.security);
    return {
      enabled,
      caPem: enabled ? tls.caPem : '',
      serverName: enabled ? tls.serverName : '',
      clientCertPem: clientCert ? tls.clientCertPem : '',
    };
  }

  private static security(dto: ConnectionDto): ConnectionSecurity {
    const wire = WireSecurityMode[dto.security.mode] ?? WireSecurityMode[SecurityModeDto.Plaintext];
    const tls = dto.security.tls;
    const stored = Object.entries(dto.security.secretFields ?? {}).filter(([, state]) => state === SecretStateDto.Set);
    return {
      ...wire,
      tls: { enabled: tls.enabled, caPem: tls.caPem ?? '', serverName: tls.serverName ?? '', clientCertPem: tls.clientCertPem ?? '' },
      username: dto.security.username ?? '',
      storedSecrets: stored.map(([field]) => field as SecretField),
      clientId: dto.security.clientId ?? '',
      tokenUrl: dto.security.tokenUrl ?? '',
      scopes: dto.security.scopes ?? [],
    };
  }

  private static env(tag: string): EnvTag {
    return (Object.values(EnvTag) as string[]).includes(tag) ? (tag as EnvTag) : EnvTag.Dev;
  }
}
