import { defineConfig, devices } from '@playwright/test';
import { BrowserEnv } from '../lib/BrowserEnv';
import { Profile } from '../lib/Target';

/** Staging demo tooling (seed + verify of the demo- resources); no acc- sweeper, nothing is cleaned up. */
const DemoSuite = { testTimeoutMs: 600_000, actionTimeoutMs: 30_000, expectTimeoutMs: 20_000, viewport: { width: 1600, height: 1000 } } as const;

export default defineConfig({
  testDir: '.',
  testMatch: /.*\.demo\.ts/,
  timeout: DemoSuite.testTimeoutMs,
  expect: { timeout: DemoSuite.expectTimeoutMs },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: '../report/demo-artifacts',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: Profile.baseURL,
    actionTimeout: DemoSuite.actionTimeoutMs,
    viewport: DemoSuite.viewport,
    screenshot: 'on',
    launchOptions: BrowserEnv.launchOptions(),
  },
});
