import type { KafkaEvent } from '../event';
import type { TransformStep } from '../transform';
import { StepHandlerRegistry } from './StepHandlers';

/** Outcome of running steps on one event; `stepIndex` is set on error or drop. */
export type TransformRunResult = {
  event?: KafkaEvent;
  dropped?: boolean;
  error?: string;
  stepIndex?: number;
  logs: string[];
};

/** Runs a transform pipeline on a copy of an event, optionally stopping at a step. */
export class TransformRunner {
  static run(input: KafkaEvent, steps: TransformStep[], uptoIndex?: number): TransformRunResult {
    let event: KafkaEvent | null = structuredClone(input);
    const logs: string[] = [];
    const count = uptoIndex == null ? steps.length : uptoIndex + 1;
    for (let i = 0; i < count; i++) {
      const step = steps[i];
      if (!step.enabled) continue;
      try {
        event = StepHandlerRegistry.get(step.kind).apply(event as KafkaEvent, step, logs);
      } catch (e) {
        return { error: (e as Error).message, stepIndex: i, event: event ?? undefined, logs };
      }
      if (event === null) return { dropped: true, stepIndex: i, logs };
    }
    return { event: event as KafkaEvent, logs };
  }
}
