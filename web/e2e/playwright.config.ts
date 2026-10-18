import { defineConfig, devices } from '@playwright/test';

/** Base URL of the web app under test (local dev stack by default). */
const E2E = { baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:5173', timeoutMs: 60_000 } as const;

export default defineConfig({
  testDir: './tests',
  timeout: E2E.timeoutMs,
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: { baseURL: E2E.baseURL, trace: 'retain-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
