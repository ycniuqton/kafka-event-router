import type { DeadLetterActionReport, DeadLetterJob, DeadLetterOriginal } from './deadLetter';

/** A header of an edited record. */
export type DeadLetterHeader = { key: string; value: string };

/** Edit of one dead letter before replay: omitted members keep the original (masked leaves left as-is are restored server side). */
export type DeadLetterEdit = { key?: unknown; value?: unknown; headers?: DeadLetterHeader[] };

/** Change kinds of a structural diff (paths only, values are never shown or audited). */
export const DiffOp = { Add: 'add', Remove: 'remove', Replace: 'replace' } as const;
export type DiffOp = (typeof DiffOp)[keyof typeof DiffOp];

export type DiffEntry = { path: string; op: DiffOp };

/** JSON-tree diff by path: `/key`, `/value/items/0`, `/headers/traceId`. */
export class StructuralDiff {
  static readonly root = { key: '/key', value: '/value', headers: '/headers' } as const;

  static of(before: { key: unknown; value: unknown; headers: DeadLetterHeader[] }, edit: DeadLetterEdit): DiffEntry[] {
    const out: DiffEntry[] = [];
    if (edit.key !== undefined) StructuralDiff.walk(StructuralDiff.root.key, before.key, edit.key, out);
    if (edit.value !== undefined) StructuralDiff.walk(StructuralDiff.root.value, before.value, edit.value, out);
    if (edit.headers) StructuralDiff.walk(StructuralDiff.root.headers, StructuralDiff.headerMap(before.headers), StructuralDiff.headerMap(edit.headers), out);
    return out;
  }

  private static headerMap(headers: DeadLetterHeader[]): Record<string, string> {
    return Object.fromEntries(headers.map((h) => [h.key, h.value]));
  }

  private static walk(path: string, a: unknown, b: unknown, out: DiffEntry[]): void {
    if (StructuralDiff.isTree(a) && StructuralDiff.isTree(b) && Array.isArray(a) === Array.isArray(b)) {
      StructuralDiff.children(path, a as Record<string, unknown>, b as Record<string, unknown>, out);
      return;
    }
    if (JSON.stringify(a) !== JSON.stringify(b)) out.push({ path, op: a === undefined ? DiffOp.Add : b === undefined ? DiffOp.Remove : DiffOp.Replace });
  }

  private static children(path: string, a: Record<string, unknown>, b: Record<string, unknown>, out: DiffEntry[]): void {
    const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])];
    keys.forEach((k) => StructuralDiff.walk(`${path}/${k.replace(/~/g, '~0').replace(/\//g, '~1')}`, a[k], b[k], out));
  }

  private static isTree(v: unknown): boolean {
    return v !== null && typeof v === 'object';
  }
}

/** Editor fields of a record. */
export const EditField = { Key: 'key', Value: 'value', Headers: 'headers' } as const;
export type EditField = (typeof EditField)[keyof typeof EditField];

/** Why a record cannot be edited in the browser. */
export const EditBlock = {
  RawBytes: 'Raw bytes cannot be edited; open the event in Transform Studio instead.',
  Schema: 'Avro and Protobuf events cannot be edited yet; replay them unchanged or fix the route.',
  Missing: 'The original event is no longer available.',
} as const;

/** Editable texts of a record: key / value as JSON text, headers as a JSON array of {key, value}. */
export type RecordEditTexts = Record<EditField, string>;

/** Pure model of the record editor: texts → validation errors → edit of the changed members only. */
export class RecordEditDraft {
  static readonly indent = 2;
  static readonly maxBytes = 1024 * 1024;
  static readonly messages = {
    json: 'Not valid JSON.',
    headers: 'Headers must be a JSON array of {"key": "…", "value": "…"} objects.',
    size: 'The edited event is larger than 1 MiB.',
  } as const;

  static blocked(original: DeadLetterOriginal | null | undefined): string | null {
    if (!original) return EditBlock.Missing;
    if (original.rawBytes) return EditBlock.RawBytes;
    return original.schema ? EditBlock.Schema : null;
  }

  static texts(original: DeadLetterOriginal): RecordEditTexts {
    const text = (v: unknown) => JSON.stringify(v ?? null, null, RecordEditDraft.indent);
    return { key: text(original.key), value: text(original.value), headers: text(original.headers) };
  }

  /** Field → message; empty when every field parses. */
  static validate(texts: RecordEditTexts): Partial<Record<EditField, string>> {
    const errors: Partial<Record<EditField, string>> = {};
    if (!RecordEditDraft.parses(texts.key)) errors.key = RecordEditDraft.messages.json;
    if (!RecordEditDraft.parses(texts.value)) errors.value = RecordEditDraft.messages.json;
    if (!RecordEditDraft.validHeaders(texts.headers)) errors.headers = RecordEditDraft.messages.headers;
    if (new Blob([texts.key, texts.value, texts.headers]).size > RecordEditDraft.maxBytes) errors.value = RecordEditDraft.messages.size;
    return errors;
  }

  /** The edit of the changed members (null when nothing changed); call only on valid texts. */
  static edit(original: DeadLetterOriginal, texts: RecordEditTexts): DeadLetterEdit | null {
    const base = RecordEditDraft.texts(original);
    const out: DeadLetterEdit = {};
    if (texts.key !== base.key) out.key = JSON.parse(texts.key);
    if (texts.value !== base.value) out.value = JSON.parse(texts.value);
    if (texts.headers !== base.headers) out.headers = JSON.parse(texts.headers) as DeadLetterHeader[];
    return Object.keys(out).length ? out : null;
  }

  private static parses(text: string): boolean {
    try {
      JSON.parse(text);
      return true;
    } catch {
      return false;
    }
  }

  private static validHeaders(text: string): boolean {
    if (!RecordEditDraft.parses(text)) return false;
    const v: unknown = JSON.parse(text);
    return Array.isArray(v) && v.every((h) => h !== null && typeof h === 'object' && typeof h.key === 'string' && typeof h.value === 'string');
  }
}

/** Outcome of a dry run through the active version (nothing produced, nothing resolved). */
export const DryRunOutcome = { WouldSend: 'would_send', Filtered: 'filtered', Failed: 'failed' } as const;
export type DryRunOutcome = (typeof DryRunOutcome)[keyof typeof DryRunOutcome];

/** Labels of dry-run outcomes. */
export const DryRunOutcomeLabel: Record<DryRunOutcome, string> = {
  [DryRunOutcome.WouldSend]: 'Would be sent',
  [DryRunOutcome.Filtered]: 'Filtered out',
  [DryRunOutcome.Failed]: 'Fails again',
};

/** One output a replay would produce. */
export type DryRunOutput = { branch: string; connectionId: string; topic: string; key: unknown; value: unknown; headers: DeadLetterHeader[] };

export type DryRunResult = { id: string; outcome: DryRunOutcome; errorType?: string; message?: string; outputs: DryRunOutput[] };

/** Dry run of a selection or group (newest ≤ 20 events). */
export type DryRunReport = { activeVersionNo: number; sampled: number; results: DryRunResult[]; counts: Partial<Record<DryRunOutcome, number>> };

/** How a single-record replay ended: preview only, replayed, or waiting for a second person. */
export const DeadLetterReplayStatus = { Preview: 'preview', Done: 'done', PendingApproval: 'pending_approval' } as const;
export type DeadLetterReplayStatus = (typeof DeadLetterReplayStatus)[keyof typeof DeadLetterReplayStatus];

export type DeadLetterReplayResult = {
  status: DeadLetterReplayStatus;
  diff: DiffEntry[];
  masked: boolean;
  activeVersionNo?: number;
  preview?: DryRunResult;
  report?: DeadLetterActionReport;
  job?: DeadLetterJob;
};

/** Options of a single-record replay. */
export type DeadLetterReplayOptions = { dryRun: boolean; note: string };

/** Options of a bulk replay job. */
export type DeadLetterJobOptions = { ids?: string[]; ratePerSecond?: number; note?: string };

/** Bounds of the bulk replay rate (events per second). */
export class ReplayRate {
  static readonly min = 1;
  static readonly max = 1000;
  static readonly default = 50;

  static parse(text: string): number | null {
    const n = Number(text);
    return Number.isInteger(n) && n >= ReplayRate.min && n <= ReplayRate.max ? n : null;
  }

  /** Rough duration of a job in seconds. */
  static seconds(count: number, rate: number): number {
    return Math.ceil(count / Math.max(rate, ReplayRate.min));
  }
}
