import type { FullConfig } from '@playwright/test';
import { Sweeper } from './Sweeper';

/** Sweeps acc- leftovers after the run (tests clean up their own resources; this catches interrupted ones). */
export default async function globalTeardown(config: FullConfig): Promise<void> {
  await Sweeper.run(config);
}
