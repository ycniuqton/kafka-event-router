import type { JsonValue } from './json';
import type { TransformStep } from './transform';

/** Kinds of library object (Go `library.ObjectKind`). */
export const LibraryKind = { Transform: 'transform', LookupTable: 'lookup_table' } as const;
export type LibraryKind = (typeof LibraryKind)[keyof typeof LibraryKind];

/** Key types of a lookup table (Go `library.ValueType`). */
export const LookupKeyType = { String: 'string', Number: 'number', Boolean: 'boolean' } as const;
export type LookupKeyType = (typeof LookupKeyType)[keyof typeof LookupKeyType];

export const LookupKeyTypeLabel: Record<LookupKeyType, string> = {
  [LookupKeyType.String]: 'Text',
  [LookupKeyType.Number]: 'Number',
  [LookupKeyType.Boolean]: 'Boolean',
};

/** A reusable, versioned transform in the library (design `LIB`); `version` is the latest label (`v3`). */
export type LibraryTransform = {
  id: string;
  name: string;
  version: string;
  latestVersion: number;
  description: string;
  usedBy: number;
  team: string;
  teamId: string;
  size: number;
  /** Row version, sent back as If-Match on metadata changes. */
  rowVersion: number;
  archived: boolean;
  updatedAt: string;
};

/** A versioned lookup table stored in the library next to transforms (spec §8.7). */
export type LibraryLookupTable = LibraryTransform & { rows: number; keyType: LookupKeyType };

/** One key → value row of a lookup table. */
export type LookupRow = { key: string; value: JsonValue };

/** One immutable version: steps of a transform or rows of a lookup table (content loaded on demand). */
export type LibraryVersion = {
  version: number;
  size: number;
  changeNote: string;
  configHash: string;
  createdAt: string;
  createdBy: string;
  steps?: TransformStep[];
  rows?: LookupRow[];
};

/** A route that references a library object, with the versions it pins. */
export type LibraryUsage = { routeId: string; routeName: string; versions: number[]; inDraft: boolean; inActive: boolean };

/** One page of lookup rows; `next` is the cursor of the following page. */
export type LookupRowsPage = { rows: LookupRow[]; total: number; next: string | null };

/** One problem of a CSV / JSON import, by 1-based line (0 = the whole file). */
export type ImportIssue = { line: number; message: string };

export type LibraryQuery = { text?: string; names?: string[] };

export type NewLibraryTransform = { name: string; description: string; steps: TransformStep[]; changeNote: string };

export type NewLookupTable = { name: string; description: string; keyType: LookupKeyType; rows: LookupRow[]; changeNote: string };

/** Rows of a new lookup table version: a grid / JSON list, or a CSV file's text. */
export type LookupRowsInput = { rows: LookupRow[] } | { csv: string };

/** Thrown by library writes refused with per-line import issues (422 `/lines/{n}`). */
export class LibraryImportError extends Error {
  readonly issues: ImportIssue[];

  constructor(issues: ImportIssue[]) {
    super(issues.map((i) => LibraryRules.issueLabel(i)).join('\n'));
    this.issues = issues;
  }
}

/** Labels and rules of library objects. */
export class LibraryRules {
  static readonly versionPrefix = 'v';
  /** Names are referenced from routes: lower-case letters, digits and dashes. */
  static readonly namePattern = /^[a-z0-9][a-z0-9-]{0,62}$/;
  static readonly maxRows = 50_000;
  static readonly maxDescription = 2000;

  static versionLabel(no: number): string {
    return `${LibraryRules.versionPrefix}${no}`;
  }

  static pinLabel(name: string, version: number): string {
    return `${name} ${LibraryRules.versionLabel(version)}`;
  }

  static stepsLabel(count: number): string {
    return `${count} step${count === 1 ? '' : 's'}`;
  }

  static usageLabel(count: number): string {
    return `${count} route${count === 1 ? '' : 's'}`;
  }

  static validName(name: string): boolean {
    return LibraryRules.namePattern.test(name);
  }

  static issueLabel(issue: ImportIssue): string {
    return issue.line > 0 ? `Line ${issue.line}: ${issue.message}` : issue.message;
  }
}

/** Steps pinned to an older library version than the latest one ("Update available"). */
export class LibraryUpdates {
  /** Latest version of `name` that is newer than `pinned`, else null. */
  static available(pinned: number | undefined, latest: number | undefined): number | null {
    if (!latest || !pinned) return null;
    return latest > pinned ? latest : null;
  }

  /** Library names referenced by steps (library steps and named lookup tables). */
  static names(steps: TransformStep[]): { transforms: string[]; tables: string[] } {
    const transforms = new Set<string>();
    const tables = new Set<string>();
    for (const s of steps) {
      if (s.libraryRef) transforms.add(s.libraryRef);
      if (s.lookupRef) tables.add(s.lookupRef);
    }
    return { transforms: [...transforms].sort(), tables: [...tables].sort() };
  }
}

/** Parses pasted / uploaded CSV text with a `key` column into rows and per-line issues (mirrors the API importer). */
export class LookupCsv {
  static readonly keyColumn = 'key';
  static readonly separator = ',';
  static readonly quote = '"';
  static readonly newline = /\r?\n/;
  static readonly bom = '﻿';

  static parse(text: string): { rows: LookupRow[]; issues: ImportIssue[] } {
    const lines = text.replace(LookupCsv.bom, '').split(LookupCsv.newline);
    const header = LookupCsv.cells(lines[0] ?? '').map((h) => h.trim());
    const keyAt = header.findIndex((h) => h.toLowerCase() === LookupCsv.keyColumn);
    if (keyAt < 0 || header.length < 2) return { rows: [], issues: [{ line: 1, message: 'The first line must be a header with a "key" column (e.g. key,value).' }] };
    const rows: LookupRow[] = [];
    const issues: ImportIssue[] = [];
    const seen = new Map<string, number>();
    lines.slice(1).forEach((line, i) => {
      if (!line.trim()) return;
      const lineNo = i + 2;
      const cells = LookupCsv.cells(line);
      const key = (cells[keyAt] ?? '').trim();
      const issue = LookupCsv.keyIssue(key, seen);
      if (issue) issues.push({ line: lineNo, message: issue });
      else {
        seen.set(key, lineNo);
        rows.push({ key, value: LookupCsv.value(header, cells, keyAt) });
      }
    });
    return { rows, issues };
  }

  /** CSV text of rows (`key,value`; values that are not text are written as JSON). */
  static format(rows: LookupRow[]): string {
    const body = rows.map((r) => [r.key, typeof r.value === 'string' ? r.value : JSON.stringify(r.value)].map((c) => LookupCsv.escape(c)).join(LookupCsv.separator));
    return [`${LookupCsv.keyColumn}${LookupCsv.separator}value`, ...body].join('\n');
  }

  private static keyIssue(key: string, seen: Map<string, number>): string | null {
    if (!key) return 'The key is empty.';
    const first = seen.get(key);
    return first ? `Duplicate key "${key}" (first seen on line ${first}).` : null;
  }

  private static value(header: string[], cells: string[], keyAt: number): JsonValue {
    const columns = header.map((name, i) => ({ name, i })).filter((c) => c.i !== keyAt);
    if (columns.length === 1) return cells[columns[0].i] ?? '';
    return Object.fromEntries(columns.map((c) => [c.name, cells[c.i] ?? '']));
  }

  private static escape(cell: string): string {
    const q = LookupCsv.quote;
    return /[",\n]/.test(cell) ? `${q}${cell.replaceAll(q, q + q)}${q}` : cell;
  }

  /** Splits one CSV line, honouring double quotes ("" inside quotes is a quote). */
  private static cells(line: string): string[] {
    const out: string[] = [];
    let cell = '';
    let quoted = false;
    for (let i = 0; i < line.length; i++) {
      const ch = line[i];
      if (quoted && ch === LookupCsv.quote && line[i + 1] === LookupCsv.quote) {
        cell += ch;
        i++;
      } else if (ch === LookupCsv.quote) quoted = !quoted;
      else if (ch === LookupCsv.separator && !quoted) {
        out.push(cell);
        cell = '';
      } else cell += ch;
    }
    out.push(cell);
    return out;
  }
}
