import { Fragment, useState, type FormEvent, type ReactNode } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ApiError, FieldErrorMapper, type AuthConfig } from '../../api/http/core';
import { Button, ButtonSize, ButtonVariant, Field, TextInput } from '../../components/ui';
import { useServiceQuery } from '../../store';
import { AuthPaths, LoginErrorText, LoginQuery } from './AuthPaths';
import { LoginMethod, LoginMethods, LoginProblem, type LoginFailure } from './LoginProblem';
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
  Email: 'Email',
  Password: 'Password',
  PasswordSubmit: 'Sign in',
  Signing: 'Signing in…',
  Loading: 'Loading sign-in options…',
  NoModes: 'No sign-in method is enabled on this server.',
} as const;

/** Form field names of the login forms (problem paths `/email`, `/password` map onto them). */
const LoginField = { Email: 'email', Password: 'password' } as const;

type MethodProps = { config: AuthConfig; returnTo: string; primary: boolean };

/** Dev login: optional email, posts `/auth/dev/login` then reloads into `return_to`. */
function DevLoginForm({ returnTo }: MethodProps) {
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
      <Field label={LoginText.DevLabel} hint={LoginText.DevHint} error={errors[LoginField.Email]}>
        <TextInput type="email" block value={email} placeholder={LoginText.DevPlaceholder} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Button type="submit" block disabled={busy}>
        {LoginText.DevSubmit}
      </Button>
    </form>
  );
}

/** Email + password: posts to `password_login_url`, then reloads into `return_to`; shows lockout / disabled / field problems. */
function PasswordLoginForm({ config, returnTo, primary }: MethodProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<LoginFailure>({ message: null, fields: {} });
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await SessionServices.current().passwordLogin({ email: email.trim(), password }, config.password_login_url);
      window.location.assign(returnTo);
    } catch (error) {
      setFailure(LoginProblem.describe(error));
      setBusy(false);
    }
  };
  return (
    <form className={styles.form} onSubmit={(e) => void submit(e)}>
      {failure.message && <div className={styles.error} role="alert">{failure.message}</div>}
      <Field label={LoginText.Email} error={failure.fields[LoginField.Email]}>
        <TextInput type="email" name={LoginField.Email} autoComplete="username" required block autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Field label={LoginText.Password} error={failure.fields[LoginField.Password]}>
        <TextInput type="password" name={LoginField.Password} autoComplete="current-password" required block value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      <Button type="submit" variant={primary ? ButtonVariant.Primary : ButtonVariant.Secondary} size={ButtonSize.Lg} block disabled={busy}>
        {busy ? LoginText.Signing : LoginText.PasswordSubmit}
      </Button>
    </form>
  );
}

/** "Sign in with SSO": full-page redirect to the IdP. */
function SsoButton({ returnTo, primary }: MethodProps) {
  return (
    <Button variant={primary ? ButtonVariant.Primary : ButtonVariant.Secondary} size={ButtonSize.Lg} block onClick={() => window.location.assign(SessionServices.current().oidcLoginUrl(returnTo))}>
      {LoginText.Sso}
    </Button>
  );
}

/** Renderer per sign-in method. */
const MethodViews: Readonly<Record<LoginMethod, (props: MethodProps) => ReactNode>> = {
  [LoginMethod.Password]: PasswordLoginForm,
  [LoginMethod.Oidc]: SsoButton,
  [LoginMethod.Dev]: DevLoginForm,
};

/** The sign-in options offered by `/auth/config`, separated by "or"; the first one is the primary action. */
function LoginOptions({ config, returnTo }: { config: AuthConfig; returnTo: string }) {
  const methods = LoginMethods.of(config);
  if (methods.length === 0) return <p className={styles.muted}>{LoginText.NoModes}</p>;
  return (
    <>
      {methods.map((method, index) => {
        const View = MethodViews[method];
        return (
          <Fragment key={method}>
            {index > 0 && <div className={styles.divider}>{LoginText.Or}</div>}
            <View config={config} returnTo={returnTo} primary={index === 0} />
          </Fragment>
        );
      })}
    </>
  );
}

/** `/login`: password form, SSO button and, on local servers, the dev login form (no sign-up: accounts come from admins). */
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
