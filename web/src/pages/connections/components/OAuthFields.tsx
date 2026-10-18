import { Field, TextInput } from '../../../components/ui';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { SecretInput } from './SecretInput';
import { TlsFields } from './TlsFields';
import { ToggleField } from './ToggleField';

/** Texts of the OAuth (SASL/OAUTHBEARER client-credentials) fields. */
const OAuthText = {
  tokenUrl: 'Token endpoint URL',
  tokenUrlHint: 'The router fetches a token with the client credentials and reuses it until it expires.',
  clientId: 'Client ID',
  scopes: 'Scopes',
  scopesHint: 'Optional, separated by spaces.',
  secret: 'Client secret — write-only after saving',
  overTls: 'Over TLS (SASL_SSL)',
} as const;

/** OAuth client credentials: token endpoint, client id, scopes, write-only client secret, optional TLS. */
export function OAuthFields(props: ConnectionFormStepProps) {
  const { form, set, errors } = props;
  const error = (field: string) => ConnectionFieldErrors.of(errors, field);
  return (
    <>
      <Field label={OAuthText.tokenUrl} hint={OAuthText.tokenUrlHint} error={error(ConnectionField.TokenUrl)}>
        <TextInput controlSize="lg" mono sunken value={form.tokenUrl} placeholder="https://idp.example.com/oauth2/token" onChange={(e) => set('tokenUrl', e.target.value)} />
      </Field>
      <Field label={OAuthText.clientId} error={error(ConnectionField.ClientId)}>
        <TextInput controlSize="lg" sunken value={form.clientId} placeholder="event-router" onChange={(e) => set('clientId', e.target.value)} />
      </Field>
      <SecretInput value={form.clientSecret} placeholder={OAuthText.secret} error={error(ConnectionField.ClientSecret)} onChange={(v) => set('clientSecret', v)} />
      <Field label={OAuthText.scopes} hint={OAuthText.scopesHint}>
        <TextInput controlSize="lg" mono sunken value={form.scopes} placeholder="kafka" onChange={(e) => set('scopes', e.target.value)} />
      </Field>
      <ToggleField label={OAuthText.overTls} checked={form.tlsEnabled} onChange={(v) => set('tlsEnabled', v)} />
      {form.tlsEnabled && <TlsFields {...props} />}
    </>
  );
}
