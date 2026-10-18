import { DeadLetterKind, DeliveryMode, type DeadLetter } from '../domain';

/** Builds the stack trace text the design derives from the error message. */
class DeadLetterSeed {
  static make(seed: Omit<DeadLetter, 'stack'>): DeadLetter {
    const stack = `TypeError: ${seed.message}\n    at transform (step-1.js:7:22)\n    at sandbox.run (runtime:1:1)`;
    return { ...seed, stack };
  }
}

const reduceError = "Cannot read properties of undefined (reading 'reduce')";

/** Design `DLS`. */
export const DEAD_LETTERS: DeadLetter[] = [
  DeadLetterSeed.make({ id: 'd1', failedAt: '2026-10-13T09:41:07Z', kind: DeadLetterKind.TransformError, message: reduceError, offset: 884162, partition: 0, routeId: 'r3', step: 'Step 1 · JavaScript', sampleId: 6 }),
  DeadLetterSeed.make({ id: 'd2', failedAt: '2026-10-13T09:38:52Z', kind: DeadLetterKind.TransformError, message: reduceError, offset: 883901, partition: 4, routeId: 'r3', step: 'Step 1 · JavaScript', sampleId: 6 }),
  DeadLetterSeed.make({ id: 'd3', failedAt: '2026-10-13T09:12:30Z', kind: DeadLetterKind.Timeout, message: 'JS step took 120 ms (limit 50 ms)', offset: 552310, partition: 2, routeId: 'r10', step: 'Step 2 · JavaScript', sampleId: 3, deliveryMode: DeliveryMode.ExactlyOnce }),
  DeadLetterSeed.make({ id: 'd4', failedAt: '2026-10-13T08:57:14Z', kind: DeadLetterKind.InvalidOutput, message: 'Field amount: expected number, got string', offset: 120044, partition: 1, routeId: 'r5', step: 'Output validation', sampleId: 9 }),
  DeadLetterSeed.make({ id: 'd5', failedAt: '2026-10-13T08:31:02Z', kind: DeadLetterKind.TransformError, message: reduceError, offset: 881230, partition: 0, routeId: 'r3', step: 'Step 1 · JavaScript', sampleId: 6 }),
];
