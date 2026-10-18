import type { FullConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { createConnection } from 'node:net';
import { join } from 'node:path';
import { Sweeper } from './Sweeper';
import { Profile } from './Target';

/** Preconditions of a run: the web app answers, the runner reaches Kafka (dev tunnel), no acc- leftovers. */
export class RunPreconditions {
  static readonly tunnelScript = join(process.cwd(), '..', '..', 'deploy', 'dev', 'tunnel.sh');
  static readonly connectMs = 3_000;

  static async check(config: FullConfig): Promise<void> {
    const res = await fetch(new URL('/auth/config', Profile.baseURL)).catch((e: Error) => e);
    if (res instanceof Error || !res.ok) throw new Error(`web app not reachable at ${Profile.baseURL}: ${res instanceof Error ? res.message : res.status}`);
    if (!(await RunPreconditions.reachable(Profile.runnerBootstrap))) RunPreconditions.startTunnel();
    if (!(await RunPreconditions.reachable(Profile.runnerBootstrap))) throw new Error(`Kafka not reachable at ${Profile.runnerBootstrap} (start deploy/dev/tunnel.sh)`);
    await Sweeper.run(config);
  }

  private static startTunnel(): void {
    if (existsSync(RunPreconditions.tunnelScript)) execFileSync(RunPreconditions.tunnelScript, ['start'], { stdio: 'inherit' });
  }

  private static reachable(hostPort: string): Promise<boolean> {
    const [host, port] = hostPort.split(':');
    return new Promise((resolve) => {
      const socket = createConnection({ host, port: Number(port), timeout: RunPreconditions.connectMs });
      socket.once('connect', () => (socket.destroy(), resolve(true)));
      socket.once('error', () => resolve(false));
      socket.once('timeout', () => (socket.destroy(), resolve(false)));
    });
  }
}

export default async function globalSetup(config: FullConfig): Promise<void> {
  await RunPreconditions.check(config);
}
