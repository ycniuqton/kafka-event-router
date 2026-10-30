import { DeadLetterKind, type DeadLetter, type DeadLetterKindCount } from './deadLetter';
import { Tone } from './tone';

/** Whose fault a dead letter is (server `record.ErrorClass`): bad input, a router bug, or a condition that heals by retrying. */
export const DeadLetterErrorClass = { Data: 'data', Router: 'router', Transient: 'transient' } as const;
export type DeadLetterErrorClass = (typeof DeadLetterErrorClass)[keyof typeof DeadLetterErrorClass];

/** Badge label, tone and explanation of each class. */
export const DeadLetterErrorClassMeta: Record<DeadLetterErrorClass, { label: string; tone: Tone; hint: string }> = {
  [DeadLetterErrorClass.Data]: { label: 'Bad data', tone: Tone.Warn, hint: 'The event or the route logic does not fit this data: fix the rule or the event, then replay.' },
  [DeadLetterErrorClass.Router]: { label: 'Router bug', tone: Tone.Err, hint: 'The router failed on its own (crash, encoding, configuration). Report it; replay after the fix.' },
  [DeadLetterErrorClass.Transient]: { label: 'Transient', tone: Tone.Info, hint: 'A dependency was unavailable or too slow. Replaying usually succeeds.' },
};

/** Classification of every error kind (mirrors internal/dlq/record/class.go; unknown kinds are router bugs). */
export class DeadLetterClasses {
  static readonly order: DeadLetterErrorClass[] = [DeadLetterErrorClass.Data, DeadLetterErrorClass.Router, DeadLetterErrorClass.Transient];

  static readonly byKind: Record<DeadLetterKind, DeadLetterErrorClass> = {
    [DeadLetterKind.TransformError]: DeadLetterErrorClass.Data,
    [DeadLetterKind.DecodeError]: DeadLetterErrorClass.Data,
    [DeadLetterKind.InvalidOutput]: DeadLetterErrorClass.Data,
    [DeadLetterKind.Timeout]: DeadLetterErrorClass.Transient,
    [DeadLetterKind.LookupUnavailable]: DeadLetterErrorClass.Transient,
    [DeadLetterKind.DependencyUnavailable]: DeadLetterErrorClass.Transient,
    [DeadLetterKind.ProduceRetriable]: DeadLetterErrorClass.Transient,
    [DeadLetterKind.InternalError]: DeadLetterErrorClass.Router,
    [DeadLetterKind.SandboxCrash]: DeadLetterErrorClass.Router,
    [DeadLetterKind.EncodeError]: DeadLetterErrorClass.Router,
    [DeadLetterKind.ProduceError]: DeadLetterErrorClass.Router,
    [DeadLetterKind.ConfigError]: DeadLetterErrorClass.Router,
    [DeadLetterKind.Unreadable]: DeadLetterErrorClass.Router,
  };

  static ofKind(kind: DeadLetterKind): DeadLetterErrorClass {
    return DeadLetterClasses.byKind[kind] ?? DeadLetterErrorClass.Router;
  }

  /** The row's class: the server's, else derived from its kind. */
  static of(d: Pick<DeadLetter, 'kind' | 'errorClass'>): DeadLetterErrorClass {
    return d.errorClass ?? DeadLetterClasses.ofKind(d.kind);
  }

  static valid(raw: string): raw is DeadLetterErrorClass {
    return (DeadLetterClasses.order as string[]).includes(raw);
  }

  /** Kinds of a class (filters translate classes into error types for older servers). */
  static kindsOf(c: DeadLetterErrorClass): DeadLetterKind[] {
    return (Object.keys(DeadLetterClasses.byKind) as DeadLetterKind[]).filter((k) => DeadLetterClasses.byKind[k] === c);
  }

  /** Counts per class from per-kind counts. */
  static counts(byKind: DeadLetterKindCount[]): Record<DeadLetterErrorClass, number> {
    const out = { [DeadLetterErrorClass.Data]: 0, [DeadLetterErrorClass.Router]: 0, [DeadLetterErrorClass.Transient]: 0 };
    byKind.forEach(({ kind, count }) => (out[DeadLetterClasses.ofKind(kind)] += count));
    return out;
  }
}
