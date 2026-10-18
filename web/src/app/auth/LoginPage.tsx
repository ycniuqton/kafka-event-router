import { useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ApiError, AuthMode, FieldErrorMapper, type AuthConfig } from '../../api/http/core';
import { Button, ButtonSize, ButtonVariant, Field, TextInput } from '../../components/ui';
import { useServiceQuery } from '../../store';
import { AuthPaths, LoginErrorText, LoginQuery } from './AuthPaths';
import { SessionServices } from './SessionServices';
import styles from './Auth.module.css';

/** Login screen copy. */
const LoginText = {
  Title: 'Sign in',
  Subtitle: 'to',
  Sso: 'Sign in with SSO',
  Or: 'or',
  DevLabel: 'Dev login email',
  DevHint: 'Leave empty to use the seeded admin.',
  DevPlaceholder: 'you@example.com',
  DevSubmit: 'Dev login',
  Loading: 'Loading sign-in options…',
  NoModes: 'No sign-in method is enabled on this server.',
} as const;

/** Form field names of the dev login form (problem paths map onto them). */
const DevLoginField = { Email: 'email' } as const;

/** Dev login: optional email, posts `/auth/dev/login` then reloads into `return_to`. */
function DevLoginForm({ returnTo }: { returnTo: string }) {
  const [email, setEmail] = useState('');
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await SessionServices.current().devLogin(email.trim() || undefined);
      window.location.assign(returnTo);
    } catch (error) {
      setErrors(error instanceof ApiError ? new FieldErrorMapper().map(error) : {});
      setBusy(false);
    }
  };
  return (
    <form className={styles.form} onSubmit={(e) => void submit(e)}>
      <Field label={LoginText.DevLabel} hint={LoginText.DevHint} error={errors[DevLoginField.Email]}>
        <TextInput type="email" block value={email} placeholder={LoginText.DevPlaceholder} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Button type="submit" block disabled={busy}>
        {LoginText.DevSubmit}
      </Button>
    </form>
  );
}

/** The sign-in options offered by `/auth/config`. */
function LoginOptions({ config, returnTo }: { config: AuthConfig; returnTo: string }) {
  const sso = config.modes.includes(AuthMode.Oidc);
  const dev = config.modes.includes(AuthMode.Dev);
  return (
    <>
      {sso && (
        <Button variant={ButtonVariant.Primary} size={ButtonSize.Lg} block onClick={() => window.location.assign(SessionServices.current().oidcLoginUrl(returnTo))}>
          {LoginText.Sso}
        </Button>
      )}
      {sso && dev && <div className={styles.divider}>{LoginText.Or}</div>}
      {dev && <DevLoginForm returnTo={returnTo} />}
      {!sso && !dev && <p className={styles.muted}>{LoginText.NoModes}</p>}
    </>
  );
}

/** `/login`: SSO button and, on local servers, the dev login form. */
export default function LoginPage() {
  const [params] = useSearchParams();
  const returnTo = AuthPaths.safeReturnTo(params.get(LoginQuery.ReturnTo));
  const callbackError = LoginErrorText.of(params.get(LoginQuery.Error));
  const { data: config, error } = useServiceQuery(() => SessionServices.current().config(), []);
  return (
    <div className={styles.screen}>
      <div className={styles.card}>
        <div>
          <h1 className={styles.title}>{LoginText.Title}</h1>
          {config && <p className={styles.subtitle}>{`${LoginText.Subtitle} ${config.workspace}`}</p>}
        </div>
        {(callbackError ?? error?.message) && <div className={styles.error}>{callbackError ?? error?.message}</div>}
        {config ? <LoginOptions config={config} returnTo={returnTo} /> : !error && <p className={styles.muted}>{LoginText.Loading}</p>}
      </div>
    </div>
  );
}
