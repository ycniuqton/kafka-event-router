import {
  DeadLetterReplayStatus,
  RecordEditDraft,
  StructuralDiff,
  type DeadLetterEdit,
  type DeadLetterOriginal,
  type DeadLetterReplayResult,
  type DiffEntry,
  type EditField,
  type RecordEditTexts,
} from '../../../domain';

/** Phase of the replay dialog. */
export const ReplayPhase = { Editing: 'editing', Busy: 'busy', Previewed: 'previewed', Done: 'done', Pending: 'pending' } as const;
export type ReplayPhase = (typeof ReplayPhase)[keyof typeof ReplayPhase];

type Fields = {
  original: DeadLetterOriginal | null;
  texts: RecordEditTexts | null;
  errors: Partial<Record<EditField, string>>;
  note: string;
  phase: ReplayPhase;
  result: DeadLetterReplayResult | null;
};

/**
 * Immutable state of the edit-then-replay dialog: editor texts and their validation, the note, the dry-run
 * preview and the final outcome (replayed, or waiting for approval). Pure transitions, unit-tested.
 */
export class ReplayDraft {
  readonly original: DeadLetterOriginal | null;
  readonly texts: RecordEditTexts | null;
  readonly errors: Partial<Record<EditField, string>>;
  readonly note: string;
  readonly phase: ReplayPhase;
  readonly result: DeadLetterReplayResult | null;

  private constructor(f: Fields) {
    this.original = f.original;
    this.texts = f.texts;
    this.errors = f.errors;
    this.note = f.note;
    this.phase = f.phase;
    this.result = f.result;
  }

  /** Editable when the original is plain JSON / text; otherwise the event is replayed unchanged. */
  static of(original: DeadLetterOriginal | null): ReplayDraft {
    const editable = original && !RecordEditDraft.blocked(original);
    return new ReplayDraft({ original, texts: editable ? RecordEditDraft.texts(original) : null, errors: {}, note: '', phase: ReplayPhase.Editing, result: null });
  }

  get blockedReason(): string | null {
    return RecordEditDraft.blocked(this.original);
  }

  get valid(): boolean {
    return Object.keys(this.errors).length === 0;
  }

  get canSubmit(): boolean {
    return this.valid && (this.phase === ReplayPhase.Editing || this.phase === ReplayPhase.Previewed);
  }

  /** The edit of changed members (null = replay unchanged). */
  get edit(): DeadLetterEdit | null {
    if (!this.original || !this.texts || !this.valid) return null;
    return RecordEditDraft.edit(this.original, this.texts);
  }

  /** Local diff before the server answers (paths only). */
  get diff(): DiffEntry[] {
    const edit = this.edit;
    if (!edit || !this.original) return [];
    return StructuralDiff.of({ key: this.original.key, value: this.original.value, headers: this.original.headers }, edit);
  }

  type(field: EditField, text: string): ReplayDraft {
    if (!this.texts) return this;
    const texts = { ...this.texts, [field]: text };
    return this.with({ texts, errors: RecordEditDraft.validate(texts), phase: ReplayPhase.Editing, result: null });
  }

  withNote(note: string): ReplayDraft {
    return this.with({ note });
  }

  busy(): ReplayDraft {
    return this.with({ phase: ReplayPhase.Busy });
  }

  /** Server answer; a failed request (null) returns to editing. */
  answered(result: DeadLetterReplayResult | null, from: ReplayPhase): ReplayDraft {
    if (!result) return this.with({ phase: from });
    const phases: Record<DeadLetterReplayStatus, ReplayPhase> = {
      [DeadLetterReplayStatus.Preview]: ReplayPhase.Previewed,
      [DeadLetterReplayStatus.Done]: ReplayPhase.Done,
      [DeadLetterReplayStatus.PendingApproval]: ReplayPhase.Pending,
    };
    return this.with({ phase: phases[result.status], result });
  }

  private with(patch: Partial<Fields>): ReplayDraft {
    return new ReplayDraft({ ...this, ...patch } as Fields);
  }
}
