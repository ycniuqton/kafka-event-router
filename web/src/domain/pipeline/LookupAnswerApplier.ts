import type { KafkaEvent } from '../event';
import { AnswerPick, LookupMissAction, LookupMockMode, LookupTraceLine, LookupTraceMode, LookupTraceResult, MockAnswer, type LookupSourceStep } from '../lookupStep';
import { StepConstant } from '../transformOptions';
import { FieldPath } from './FieldPath';

type Payload = Record<string, unknown>;

/** Thrown for `onMiss: fail` so the in-browser runner reports a step error like the engine's `lookup_miss`. */
export class LookupMissError extends Error {
  static readonly text = 'lookup_miss: the source has no answer for this event';

  constructor() {
    super(LookupMissError.text);
    this.name = 'LookupMissError';
  }
}

/** Applies a lookup source step in the browser: the mock answer through the mapping, a mocked miss through onMiss. */
export class LookupAnswerApplier {
  static apply(event: KafkaEvent, step: LookupSourceStep, logs: string[]): KafkaEvent | null {
    if (step.mockMode === LookupMockMode.Off) return event;
    const answer = step.mockMode === LookupMockMode.Hit ? MockAnswer.parse(step.mock) : undefined;
    const hit = answer !== undefined;
    logs.push(LookupTraceLine.format({ mode: LookupTraceMode.Mock, source: step.sourceId, result: hit ? LookupTraceResult.Hit : LookupTraceResult.Miss, ms: 0 }));
    if (hit) return LookupAnswerApplier.write(event, step, (from) => AnswerPick.read(answer, from));
    return LookupAnswerApplier.miss(event, step);
  }

  private static miss(event: KafkaEvent, step: LookupSourceStep): KafkaEvent | null {
    if (step.onMiss.action === LookupMissAction.Drop) return null;
    if (step.onMiss.action === LookupMissAction.Fail) throw new LookupMissError();
    if (step.onMiss.action !== LookupMissAction.Default) return event;
    const value = StepConstant.parse(step.onMiss.value);
    return LookupAnswerApplier.write(event, step, () => value);
  }

  private static write(event: KafkaEvent, step: LookupSourceStep, valueOf: (from: string) => unknown): KafkaEvent {
    const payload = event.value as Payload;
    for (const row of step.map.filter((m) => m.to.trim())) {
      const value = valueOf(row.from.trim());
      if (value !== undefined) FieldPath.set(payload, row.to.trim(), structuredClone(value));
    }
    return event;
  }
}
