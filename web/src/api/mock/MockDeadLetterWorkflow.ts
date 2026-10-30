import {
  DeadLetterClasses,
  DeadLetterErrorClass,
  DryRunOutcome,
  EnvTag,
  type DeadLetter,
  type DeadLetterEdit,
  type DeadLetterOriginal,
  type DryRunResult,
} from '../../domain';
import type { MockBackend } from './MockBackend';

/** Mock strict 4-eyes: replays of routes writing to a `prod` connection need a second person's approval. */
export class MockFourEyes {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    this.backend = backend;
  }

  requiresApproval(routeId: string): boolean {
    const route = this.backend.db.routes.find((r) => r.id === routeId);
    return this.backend.db.connections.find((c) => c.id === route?.destination.connectionId)?.env === EnvTag.Prod;
  }
}

/**
 * Mock dry run through the "active version": an unedited event fails again with its own error unless its cause is
 * transient (a dependency healed); an edited event, or a transient one, would be sent to the route destination.
 */
export class MockDryRunner {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    this.backend = backend;
  }

  run(d: DeadLetter, original: DeadLetterOriginal | null, edit: DeadLetterEdit | null): DryRunResult {
    const transient = DeadLetterClasses.of(d) === DeadLetterErrorClass.Transient;
    if (!edit && !transient) return { id: d.id, outcome: DryRunOutcome.Failed, errorType: d.kind, message: d.message, outputs: [] };
    const route = this.backend.db.routes.find((r) => r.id === d.routeId);
    const output = {
      branch: 'main',
      connectionId: route?.destination.connectionId ?? '',
      topic: route?.destination.topic ?? '',
      key: edit?.key ?? original?.key ?? null,
      value: edit?.value ?? original?.value ?? null,
      headers: edit?.headers ?? original?.headers ?? [],
    };
    return { id: d.id, outcome: DryRunOutcome.WouldSend, outputs: [output] };
  }
}
