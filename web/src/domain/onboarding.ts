/** The team's demo connection ("Try with demo data", `POST /onboarding/demo`). */
export type DemoSeed = { connectionId: string; topic: string };

/** Server first-run state (`GET /onboarding`): demo availability and the seeded demo, if any. */
export type OnboardingState = { allDone: boolean; demoAvailable: boolean; demo: DemoSeed | null };

/** Copy and defaults of the demo data shortcut. */
export const DemoDataText = {
  topic: 'order-events',
  created: 'Demo connection ready — order-events is filling with sample orders',
  removed: 'Demo data removed',
  exists: 'Demo data already exists',
} as const;
