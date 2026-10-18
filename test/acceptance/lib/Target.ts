import { existsSync, readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

/** Environments the acceptance suite can run against (ACC_TARGET). */
export enum AccTarget {
  Local = 'local',
  Staging = 'staging',
}

/** How the suite signs in to the web app / API. */
export enum LoginMethod {
  Dev = 'dev',
  DexOidc = 'dex_oidc',
}

/** Environment variable names the suite reads. */
export const AccEnv = {
  target: 'ACC_TARGET',
  baseURL: 'ACC_BASE_URL',
  platformBootstrap: 'ACC_PLATFORM_BOOTSTRAP',
  runnerBootstrap: 'ACC_RUNNER_BOOTSTRAP',
  secretsFile: 'ACC_SECRETS_FILE',
  playwrightLibs: 'ACC_PLAYWRIGHT_LIBS',
} as const;

/** Keys read (one at a time, never sourced) from the staging credentials file. */
export const SecretKey = {
  stagingUrl: 'STAGING_URL',
  stagingHost: 'STAGING_HOST',
  loginEmail: 'STAGING_LOGIN_EMAIL',
  loginPassword: 'STAGING_LOGIN_PASSWORD',
} as const;

/** Reads single KEY=value lines from the credentials file without evaluating them. */
export class SecretsFile {
  static readonly defaultPath = join(homedir(), '.secrets', 'event-router-staging.env');

  constructor(private readonly path: string = process.env[AccEnv.secretsFile] ?? SecretsFile.defaultPath) {}

  /** Value of key, or '' when the file or key is missing. */
  get(key: string): string {
    if (!existsSync(this.path)) return '';
    const line = readFileSync(this.path, 'utf8').split('\n').find((l) => l.startsWith(`${key}=`));
    return SecretsFile.unquote(line ? line.slice(key.length + 1).trim() : '');
  }

  private static unquote(value: string): string {
    const quoted = value.length >= 2 && ['"', "'"].includes(value[0]) && value.at(-1) === value[0];
    return quoted ? value.slice(1, -1) : value;
  }
}

/** Everything that differs between targets: URLs, login, and Kafka addresses as seen by platform and runner. */
export class TargetProfile {
  constructor(
    readonly target: AccTarget,
    readonly baseURL: string,
    readonly login: LoginMethod,
    readonly platformBootstrap: string,
    readonly runnerBootstrap: string,
    readonly credentials: { email: string; password: string },
  ) {}

  /** Multi-user scenarios (viewer/editor/approver) need dev login with arbitrary emails. */
  get canImpersonate(): boolean {
    return this.login === LoginMethod.Dev;
  }

  static current(): TargetProfile {
    const target = (process.env[AccEnv.target] ?? AccTarget.Local) as AccTarget;
    return target === AccTarget.Staging ? TargetProfile.staging() : TargetProfile.local();
  }

  private static local(): TargetProfile {
    return new TargetProfile(
      AccTarget.Local,
      process.env[AccEnv.baseURL] ?? 'http://localhost:5173',
      LoginMethod.Dev,
      process.env[AccEnv.platformBootstrap] ?? '127.0.0.1:29092',
      process.env[AccEnv.runnerBootstrap] ?? '127.0.0.1:29092',
      { email: '', password: '' },
    );
  }

  private static staging(): TargetProfile {
    const secrets = new SecretsFile();
    const host = secrets.get(SecretKey.stagingHost);
    const url = process.env[AccEnv.baseURL] ?? (secrets.get(SecretKey.stagingUrl) || `http://${host}`);
    return new TargetProfile(
      AccTarget.Staging,
      url.replace(/\/$/, ''),
      LoginMethod.DexOidc,
      process.env[AccEnv.platformBootstrap] ?? 'kafka:19092',
      process.env[AccEnv.runnerBootstrap] ?? '127.0.0.1:29092',
      { email: secrets.get(SecretKey.loginEmail), password: secrets.get(SecretKey.loginPassword) },
    );
  }
}

/** The profile of this run (resolved once per worker process). */
export const Profile = TargetProfile.current();
