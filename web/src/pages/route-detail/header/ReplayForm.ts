import { ApiError } from '../../../api/http/core';
import type { Replay, ReplayRequest } from '../../../domain';

/** Fields of the replay dialog. */
export const ReplayField = { From: 'from', Name: 'name', Ack: 'ack', Form: 'form' } as const;
export type ReplayField = (typeof ReplayField)[keyof typeof ReplayField];

export type ReplayInput = { start: string; acknowledged: boolean; typedName: string };
export type ReplayErrors = Partial<Record<ReplayField, string>>;
export type ReplayOutcome = { replay: Replay; errors: null } | { replay: null; errors: ReplayErrors };
export type ReplayStarter = (request: ReplayRequest) => Promise<Replay>;

/** Copy of the replay dialog. */
export const ReplayText = {
  fromRequired: 'Pick the date and time to replay from.',
  fromFuture: 'The replay start is in the future. Pick a time in the past.',
  nameMismatch: 'Type the route name exactly to confirm.',
  ackRequired: 'Confirm that the destination will receive duplicates.',
  started: (destination: string) => `Replay started → ${destination}`,
  done: 'Replay done — the runners are reading again from the chosen time',
  failed: (error: string) => `Replay failed: ${error}`,
} as const;

/** Validation and request building of the replay confirm dialog (timestamp in the past, ack, typed route name). */
export class ReplayForm {
  static readonly lookbackMs = 3_600_000;
  static readonly inputLength = 16;
  static readonly utcSuffix = ':00Z';

  static defaultStart(now: Date): string {
    return new Date(now.getTime() - ReplayForm.lookbackMs).toISOString().slice(0, ReplayForm.inputLength);
  }

  /** ISO instant of a `datetime-local` value read as UTC (null when empty or invalid). */
  static instant(start: string): string | null {
    if (!start) return null;
    const at = new Date(`${start.slice(0, ReplayForm.inputLength)}${ReplayForm.utcSuffix}`);
    return Number.isNaN(at.getTime()) ? null : at.toISOString();
  }

  static validate(input: ReplayInput, now: Date, routeName: string): ReplayErrors {
    const errors: ReplayErrors = {};
    const from = ReplayForm.instant(input.start);
    if (!from) errors.from = ReplayText.fromRequired;
    else if (new Date(from).getTime() > now.getTime()) errors.from = ReplayText.fromFuture;
    if (!input.acknowledged) errors.ack = ReplayText.ackRequired;
    if (input.typedName.trim() !== routeName) errors.name = ReplayText.nameMismatch;
    return errors;
  }

  static canSubmit(input: ReplayInput, now: Date, routeName: string): boolean {
    return Object.keys(ReplayForm.validate(input, now, routeName)).length === 0;
  }

  static request(input: ReplayInput): ReplayRequest {
    return { from: ReplayForm.instant(input.start) ?? '', confirm: true };
  }

  /** Validates, then calls the replay API; client or server problems come back as field errors. */
  static async submit(input: ReplayInput, now: Date, routeName: string, start: ReplayStarter): Promise<ReplayOutcome> {
    const errors = ReplayForm.validate(input, now, routeName);
    if (Object.keys(errors).length > 0) return { replay: null, errors };
    try {
      return { replay: await start(ReplayForm.request(input)), errors: null };
    } catch (e) {
      return { replay: null, errors: ReplayForm.serverErrors(e) };
    }
  }

  /** Server problems: `/from` lands on the time field, everything else on the form. */
  static serverErrors(error: unknown): ReplayErrors {
    if (error instanceof ApiError) {
      const from = error.fieldErrors()['/from'];
      if (from) return { from };
    }
    return { form: (error as Error).message };
  }
}
