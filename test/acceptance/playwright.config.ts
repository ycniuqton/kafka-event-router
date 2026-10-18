import { defineConfig, devices } from '@playwright/test';
import { BrowserEnv } from './lib/BrowserEnv';
import { Profile } from './lib/Target';

/** Suite-wide settings: timeouts, report location, viewport. */
const Suite = {
  testTimeoutMs: 180_000,
  actionTimeoutMs: 30_000,
  expectTimeoutMs: 15_000,
  reportDir: 'report',
  viewport: { width: 1600, height: 1000 },
} as const;

export default defineConfig({
  testDir: './specs',
  globalSetup: './lib/GlobalSetup.ts',
  globalTeardown: './lib/GlobalTeardown.ts',
  timeout: Suite.testTimeoutMs,
  expect: { timeout: Suite.expectTimeoutMs },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [
    ['list'],
    ['html', { outputFolder: `${Suite.reportDir}/html`, open: 'never' }],
    ['json', { outputFile: `${Suite.reportDir}/results.json` }],
  ],
  outputDir: `${Suite.reportDir}/artifacts`,
  metadata: { target: Profile.target, baseURL: Profile.baseURL },
  use: {
    baseURL: Profile.baseURL,
    actionTimeout: Suite.actionTimeoutMs,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    viewport: Suite.viewport,
    launchOptions: BrowserEnv.launchOptions(),
  },
  projects: [
    { name: 'api', testDir: './specs/api', use: { ...devices['Desktop Chrome'], viewport: Suite.viewport } },
    { name: 'ui', testDir: './specs/ui', use: { ...devices['Desktop Chrome'], viewport: Suite.viewport } },
  ],
});
