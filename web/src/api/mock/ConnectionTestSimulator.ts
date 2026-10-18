import {
  ConnectionStatus,
  ConnectionTestChecks,
  TestStepState,
  type Connection,
  type ConnectionTestRun,
  type ConnectionTestStep,
} from '../../domain';

/** Reproduces the design's scripted test: auth failures stop at step 2, unreachable at step 1. */
export class ConnectionTestSimulator {
  static readonly stepCount = ConnectionTestChecks.length;
  private readonly connection: Connection;
  readonly failAt: number | null;

  constructor(connection: Connection) {
    this.connection = connection;
    this.failAt = ConnectionTestSimulator.failureIndex(connection.status);
  }

  private static failureIndex(status: ConnectionStatus): number | null {
    if (status === ConnectionStatus.AuthFailed) return 1;
    return status === ConnectionStatus.Unreachable ? 0 : null;
  }

  isFinished(progress: number): boolean {
    return progress >= ConnectionTestSimulator.stepCount || (this.failAt !== null && progress > this.failAt);
  }

  snapshot(progress: number): ConnectionTestRun {
    const finished = this.isFinished(progress);
    return {
      connectionId: this.connection.id,
      steps: ConnectionTestChecks.map((label, i) => this.step(label, i, progress)),
      finished,
      ok: finished && this.failAt === null,
    };
  }

  private step(label: string, index: number, progress: number): ConnectionTestStep {
    const skipped = this.failAt !== null && index > this.failAt;
    if (index < progress && index === this.failAt) return { label, state: TestStepState.Failed, reason: this.reason(index) };
    if (index < progress) return { label, state: TestStepState.Passed };
    if (skipped) return { label, state: TestStepState.Skipped };
    const running = index === progress && progress < ConnectionTestSimulator.stepCount;
    return { label, state: running ? TestStepState.Running : TestStepState.Pending };
  }

  private reason(index: number): string {
    if (index === 0) {
      return `Can't reach broker ${this.connection.bootstrapServers[0]}. The server may be down or blocked by a firewall. Check network rules, then test again.`;
    }
    return 'Authentication failed. Check the SASL mechanism: the broker expects SCRAM-SHA-512, this connection uses mTLS.';
  }
}
