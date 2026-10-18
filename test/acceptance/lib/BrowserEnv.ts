import { existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { AccEnv } from './Target';

/** Chromium may need system libraries unpacked without sudo (see web/e2e/README.md); they are added to LD_LIBRARY_PATH. */
export class BrowserEnv {
  static readonly libs = process.env[AccEnv.playwrightLibs] ?? join(homedir(), '.local', 'lib', 'playwright-deps', 'lib');

  static launchOptions(): { env?: Record<string, string | undefined> } {
    if (!existsSync(BrowserEnv.libs)) return {};
    return { env: { ...process.env, LD_LIBRARY_PATH: [BrowserEnv.libs, process.env.LD_LIBRARY_PATH].filter(Boolean).join(':') } };
  }
}
