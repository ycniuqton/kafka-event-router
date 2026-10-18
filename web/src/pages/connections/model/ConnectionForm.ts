import {
  ConnectionDisplay,
  EnvTag,
  RegistryAuthMode,
  ScramMechanism,
  SecretField,
  SecurityMode,
  SecurityModeRules,
  type Connection,
  type ConnectionDraft,
  type ConnectionSecurity,
} from '../../../domain';
import { BootstrapListParser } from './BootstrapListParser';

export { ScramMechanism } from '../../../domain';

/** The four steps of the create / edit connection panel (spec §5.2). */
export const ConnectionFormStep = { Basics: 0, Brokers: 1, Security: 2, SchemaRegistry: 3 } as const;
export type ConnectionFormStep = (typeof ConnectionFormStep)[keyof typeof ConnectionFormStep];

export const ConnectionFormStepLabels = ['Basics', 'Brokers', 'Security', 'Schema registry'] as const;

/** A raw client property row in the Advanced expander. */
export type ClientProperty = { id: number; key: string; value: string };

/** A write-only secret: `stored` means a value is saved server-side and is never shown again. */
export type SecretValue = { value: string; stored: boolean };

/** Everything the panel edits. */
export type ConnectionFormState = {
  name: string;
  description: string;
  env: EnvTag;
  team: string;
  allowTopicCreate: boolean;
  probeEnabled: boolean;
  sensitive: boolean;
  servers: string[];
  brokerInput: string;
  security: SecurityMode;
  scram: ScramMechanism;
  tlsEnabled: boolean;
  caPem: string;
  serverName: string;
  clientCertPem: string;
  clientKey: SecretValue;
  username: string;
  password: SecretValue;
  tokenUrl: string;
  clientId: string;
  /** OAuth scopes as typed (space or comma separated). */
  scopes: string;
  clientSecret: SecretValue;
  registryUrl: string;
  registryAuth: RegistryAuthMode;
  registryUser: string;
  /** Registry password (basic) or token (bearer). */
  registryPassword: SecretValue;
  properties: ClientProperty[];
};

/** Defaults the design pre-fills in a new connection form. */
export const ConnectionFormDefaults = {
  env: EnvTag.Staging,
  team: 'Platform',
  security: SecurityMode.SaslScram,
  scram: ScramMechanism.Sha512,
  property: { id: 1, key: 'request.timeout.ms', value: '30000' },
} as const;

/** Validation and toast messages copied from the design. */
export const ConnectionFormMessages = {
  nameRequired: 'Give the connection a name',
  nameTaken: 'A connection with this name already exists',
  serversRequired: 'Add at least one bootstrap server',
  serversInvalid: 'Fix the red bootstrap servers: host:port expected',
  updated: 'Connection updated',
} as const;

/** Builds, maps and validates the connection form state. */
export class ConnectionFormModel {
  static readonly emptySecret: SecretValue = { value: '', stored: false };
  static readonly storedSecret: SecretValue = { value: '', stored: true };
  private static readonly nameSpaces = /\s/g;
  private static readonly nameSeparator = '-';

  static empty(team: string = ConnectionFormDefaults.team): ConnectionFormState {
    return {
      name: '',
      description: '',
      env: ConnectionFormDefaults.env,
      team,
      allowTopicCreate: false,
      probeEnabled: true,
      sensitive: false,
      servers: [],
      brokerInput: '',
      security: ConnectionFormDefaults.security,
      scram: ConnectionFormDefaults.scram,
      tlsEnabled: false,
      caPem: '',
      serverName: '',
      clientCertPem: '',
      clientKey: ConnectionFormModel.emptySecret,
      username: '',
      password: ConnectionFormModel.emptySecret,
      tokenUrl: '',
      clientId: '',
      scopes: '',
      clientSecret: ConnectionFormModel.emptySecret,
      registryUrl: '',
      registryAuth: RegistryAuthMode.None,
      registryUser: '',
      registryPassword: ConnectionFormModel.emptySecret,
      properties: [{ ...ConnectionFormDefaults.property }],
    };
  }

  /** Pre-fills the form from a saved connection; secrets come back as stored placeholders. */
  static fromConnection(connection: Connection): ConnectionFormState {
    const security = connection.securityDetail ?? SecurityLabel.toSecurity(connection.security);
    const secret = (field: SecretField) => (security.storedSecrets.includes(field) ? ConnectionFormModel.storedSecret : ConnectionFormModel.emptySecret);
    const registry = connection.schemaRegistry;
    return {
      ...ConnectionFormModel.empty(connection.owner),
      name: ConnectionDisplay.name(connection),
      description: connection.description ?? '',
      env: connection.env,
      allowTopicCreate: connection.allowTopicCreate ?? false,
      probeEnabled: connection.probeEnabled ?? true,
      sensitive: connection.sensitive ?? false,
      servers: [...connection.bootstrapServers],
      security: security.mode,
      scram: security.scram,
      tlsEnabled: security.tls.enabled,
      caPem: security.tls.caPem,
      serverName: security.tls.serverName,
      clientCertPem: security.tls.clientCertPem,
      username: security.username,
      password: secret(SecretField.Password),
      clientKey: secret(SecretField.ClientKeyPem),
      clientSecret: secret(SecretField.ClientSecret),
      clientId: security.clientId ?? '',
      tokenUrl: security.tokenUrl ?? '',
      scopes: OAuthScopes.join(security.scopes ?? []),
      registryUrl: registry?.url ?? '',
      registryAuth: registry?.authMode ?? RegistryAuthMode.None,
      registryUser: registry?.username ?? '',
      registryPassword: registry?.secretStored ? ConnectionFormModel.storedSecret : ConnectionFormModel.emptySecret,
      properties: ClientProperties.fromMap(connection.properties ?? {}),
    };
  }

  /** Lower-case, dash-separated names, as typed in the design. */
  static normalizeName(raw: string): string {
    return raw.replace(ConnectionFormModel.nameSpaces, ConnectionFormModel.nameSeparator).toLowerCase();
  }

  static toDraft(form: ConnectionFormState): ConnectionDraft {
    return {
      name: form.name,
      description: form.description,
      env: form.env,
      team: form.team,
      bootstrapServers: BootstrapListParser.add(form.servers, form.brokerInput),
      security: form.security,
      scram: form.scram,
      tls: { enabled: form.tlsEnabled, caPem: form.caPem, serverName: form.serverName, clientCertPem: form.clientCertPem },
      username: form.username,
      password: form.password.value,
      clientKeyPem: form.clientKey.value,
      clientId: form.clientId.trim(),
      tokenUrl: form.tokenUrl.trim(),
      scopes: OAuthScopes.split(form.scopes),
      clientSecret: form.clientSecret.value,
      schemaRegistryUrl: form.registryUrl,
      schemaRegistryAuth: form.registryAuth,
      schemaRegistryUsername: form.registryUser,
      schemaRegistrySecret: form.registryPassword.value,
      properties: ClientProperties.toMap(form.properties),
      allowTopicCreate: form.allowTopicCreate,
      probeEnabled: form.probeEnabled,
      sensitive: form.sensitive,
    };
  }

  /** Message blocking "Next" from the given step, or null when it may advance. */
  static blocker(form: ConnectionFormState, step: number, takenNames: string[]): string | null {
    if (step === ConnectionFormStep.Basics && !form.name) return ConnectionFormMessages.nameRequired;
    if (step === ConnectionFormStep.Basics && takenNames.includes(form.name)) return ConnectionFormMessages.nameTaken;
    if (step === ConnectionFormStep.Brokers && !form.servers.length && !form.brokerInput) return ConnectionFormMessages.serversRequired;
    if (step === ConnectionFormStep.Brokers && BootstrapListParser.invalid(form.servers).length) return ConnectionFormMessages.serversInvalid;
    return null;
  }
}

/** OAuth scopes typed as one line ↔ the list sent to the server. */
export class OAuthScopes {
  private static readonly separators = /[\s,]+/;
  private static readonly joiner = ' ';

  static split(raw: string): string[] {
    return raw.split(OAuthScopes.separators).filter(Boolean);
  }

  static join(scopes: string[]): string {
    return scopes.join(OAuthScopes.joiner);
  }
}

/** Maps the security labels of mock clusters (e.g. "SASL/SCRAM-512") back to panel settings. */
export class SecurityLabel {
  private static readonly sha256Marker = '256';

  static toMode(label: string): SecurityMode {
    const modes = Object.values(SecurityMode).sort((a, b) => b.length - a.length);
    return modes.find((m) => label.startsWith(m)) ?? SecurityMode.None;
  }

  static toSecurity(label: string): ConnectionSecurity {
    const mode = SecurityLabel.toMode(label);
    const scram = label.endsWith(SecurityLabel.sha256Marker) ? ScramMechanism.Sha256 : ScramMechanism.Sha512;
    const tls = { enabled: SecurityModeRules.tlsEnabled(mode, false), caPem: '', serverName: '', clientCertPem: '' };
    return { mode, scram, tls, username: '', storedSecrets: [...SecurityModeRules.secrets[mode]] };
  }
}

/** Client properties the server accepts (mirror of Go `kafka.PropertyCatalog`). */
export const AllowedClientProperties = [
  'client.id',
  'request.timeout.ms',
  'metadata.max.age.ms',
  'connections.max.idle.ms',
  'fetch.max.bytes',
  'socket.connection.setup.timeout.ms',
] as const;

/** Editing of the Advanced raw client-property rows. */
export class ClientProperties {
  static add(rows: ClientProperty[]): ClientProperty[] {
    const id = rows.reduce((max, r) => Math.max(max, r.id), 0) + 1;
    return [...rows, { id, key: '', value: '' }];
  }

  static update(rows: ClientProperty[], id: number, patch: Partial<Omit<ClientProperty, 'id'>>): ClientProperty[] {
    return rows.map((r) => (r.id === id ? { ...r, ...patch } : r));
  }

  static remove(rows: ClientProperty[], id: number): ClientProperty[] {
    return rows.filter((r) => r.id !== id);
  }

  static fromMap(map: Record<string, string>): ClientProperty[] {
    return Object.entries(map).map(([key, value], i) => ({ id: i + 1, key, value }));
  }

  /** Rows with a key, trimmed; the last row wins for duplicate keys. */
  static toMap(rows: ClientProperty[]): Record<string, string> {
    return Object.fromEntries(rows.filter((r) => r.key.trim()).map((r) => [r.key.trim(), r.value.trim()]));
  }
}
