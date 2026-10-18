import { ApiError, FieldErrorMapper, JsonPointer, ProblemKind } from '../../../api/http/core';
import { ConnectionFormStep } from './ConnectionForm';

/** Form field names keyed in `ConnectionFieldErrors.fields` (also used by the step views). */
export const ConnectionField = {
  Name: 'name',
  Description: 'description',
  Env: 'env',
  Team: 'team',
  Servers: 'bootstrapServers',
  Security: 'security',
  Username: 'username',
  Password: 'password',
  CaPem: 'caPem',
  ServerName: 'serverName',
  ClientCertPem: 'clientCertPem',
  ClientKey: 'clientKey',
  ClientId: 'clientId',
  TokenUrl: 'tokenUrl',
  ClientSecret: 'clientSecret',
  RegistryUrl: 'registryUrl',
  RegistryAuth: 'registryAuth',
  RegistryUser: 'registryUser',
  RegistrySecret: 'registrySecret',
  Properties: 'properties',
  ReadTopic: 'readTopic',
  WriteTopic: 'writeTopic',
} as const;
export type ConnectionField = (typeof ConnectionField)[keyof typeof ConnectionField];

/** API JSON Pointers → panel fields (unaliased pointers keep their dotted form, e.g. `bootstrapServers.2`). */
const ConnectionFieldAliases = {
  '/envTag': ConnectionField.Env,
  '/owner': ConnectionField.Team,
  '/security/mode': ConnectionField.Security,
  '/security/username': ConnectionField.Username,
  '/security/tls/caPem': ConnectionField.CaPem,
  '/security/tls/serverName': ConnectionField.ServerName,
  '/security/tls/clientCertPem': ConnectionField.ClientCertPem,
  '/security/tls': ConnectionField.Security,
  '/secrets/password': ConnectionField.Password,
  '/secrets/clientKeyPem': ConnectionField.ClientKey,
  '/security/clientId': ConnectionField.ClientId,
  '/security/tokenUrl': ConnectionField.TokenUrl,
  '/secrets/clientSecret': ConnectionField.ClientSecret,
  '/schemaRegistry/url': ConnectionField.RegistryUrl,
  '/schemaRegistry/auth/mode': ConnectionField.RegistryAuth,
  '/schemaRegistry/auth/username': ConnectionField.RegistryUser,
  '/schemaRegistry': ConnectionField.RegistryUrl,
  '/secrets/schemaRegistrySecret': ConnectionField.RegistrySecret,
} as const;

/** Panel step that holds each field (first segment of the field name). */
const FieldStep: Record<string, ConnectionFormStep> = {
  [ConnectionField.Name]: ConnectionFormStep.Basics,
  [ConnectionField.Description]: ConnectionFormStep.Basics,
  [ConnectionField.Env]: ConnectionFormStep.Basics,
  [ConnectionField.Team]: ConnectionFormStep.Basics,
  [ConnectionField.Servers]: ConnectionFormStep.Brokers,
  [ConnectionField.Security]: ConnectionFormStep.Security,
  [ConnectionField.Username]: ConnectionFormStep.Security,
  [ConnectionField.Password]: ConnectionFormStep.Security,
  [ConnectionField.CaPem]: ConnectionFormStep.Security,
  [ConnectionField.ServerName]: ConnectionFormStep.Security,
  [ConnectionField.ClientCertPem]: ConnectionFormStep.Security,
  [ConnectionField.ClientKey]: ConnectionFormStep.Security,
  [ConnectionField.ClientId]: ConnectionFormStep.Security,
  [ConnectionField.TokenUrl]: ConnectionFormStep.Security,
  [ConnectionField.ClientSecret]: ConnectionFormStep.Security,
  [ConnectionField.RegistryUrl]: ConnectionFormStep.SchemaRegistry,
  [ConnectionField.RegistryAuth]: ConnectionFormStep.SchemaRegistry,
  [ConnectionField.RegistryUser]: ConnectionFormStep.SchemaRegistry,
  [ConnectionField.RegistrySecret]: ConnectionFormStep.SchemaRegistry,
  [ConnectionField.Properties]: ConnectionFormStep.SchemaRegistry,
};

/** Field errors of a failed save / draft test and the step to jump to (CN-48); `stale` = 412, reload first. */
export type ConnectionFieldErrorView = { fields: Record<string, string>; step: ConnectionFormStep | null; message: string; stale: boolean };

/** Maps RFC 7807 problems onto panel fields and steps; other errors only carry a message. */
export class ConnectionFieldErrors {
  static readonly none: ConnectionFieldErrorView = { fields: {}, step: null, message: '', stale: false };
  private static readonly mapper = new FieldErrorMapper(ConnectionFieldAliases);

  static from(error: unknown): ConnectionFieldErrorView {
    const message = error instanceof Error ? error.message : String(error);
    if (!(error instanceof ApiError)) return { fields: {}, step: null, message, stale: false };
    const fields = ConnectionFieldErrors.mapper.map(error);
    const first = Object.keys(fields)[0];
    const step = first === undefined ? null : ConnectionFieldErrors.stepOf(first);
    return { fields, step, message, stale: error.is(ProblemKind.PreconditionFailed) };
  }

  /** Message for a field and its children (`properties` matches `properties.foo`). */
  static of(fields: Record<string, string>, field: string): string | undefined {
    const prefix = field + JsonPointer.fieldSeparator;
    return fields[field] ?? Object.entries(fields).find(([k]) => k.startsWith(prefix))?.[1];
  }

  /** Message of one property row by key (`/properties/<key>`). */
  static ofProperty(fields: Record<string, string>, key: string): string | undefined {
    return fields[ConnectionField.Properties + JsonPointer.fieldSeparator + key];
  }

  private static stepOf(field: string): ConnectionFormStep | null {
    const head = field.split(JsonPointer.fieldSeparator)[0];
    return FieldStep[head] ?? null;
  }
}
