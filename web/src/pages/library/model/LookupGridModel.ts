import { LookupCsv, type ImportIssue, type JsonValue, type LookupRow } from '../../../domain';

/** One editable grid row: key text and value text (JSON literal or plain text). */
export type GridRow = { key: string; value: string };

/**
 * Editable lookup rows for the grid editor: cells are text, values that parse as JSON keep their type.
 * Pasting CSV (with a key header) replaces or appends rows. issues() lists empty and duplicate keys by line.
 */
export class LookupGridModel {
  readonly rows: GridRow[];

  constructor(rows: GridRow[]) {
    this.rows = rows;
  }

  static fromRows(rows: LookupRow[]): LookupGridModel {
    return new LookupGridModel(rows.map((r) => ({ key: r.key, value: LookupGridModel.cell(r.value) })));
  }

  static cell(value: JsonValue): string {
    return typeof value === 'string' ? value : JSON.stringify(value);
  }

  static parseValue(text: string): JsonValue {
    const trimmed = text.trim();
    if (!trimmed || !/^[[{"\d\-tfn]/.test(trimmed)) return text;
    try {
      return JSON.parse(trimmed) as JsonValue;
    } catch {
      return text;
    }
  }

  set(index: number, patch: Partial<GridRow>): LookupGridModel {
    return new LookupGridModel(this.rows.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  }

  add(): LookupGridModel {
    return new LookupGridModel([...this.rows, { key: '', value: '' }]);
  }

  remove(index: number): LookupGridModel {
    return new LookupGridModel(this.rows.filter((_, i) => i !== index));
  }

  /** Applies pasted CSV text: rows with an existing key are replaced, new keys appended. */
  paste(csv: string): { grid: LookupGridModel; issues: ImportIssue[] } {
    const parsed = LookupCsv.parse(csv);
    const next = [...this.rows];
    for (const row of parsed.rows) {
      const cell = { key: row.key, value: LookupGridModel.cell(row.value) };
      const at = next.findIndex((r) => r.key === row.key);
      if (at >= 0) next[at] = cell;
      else next.push(cell);
    }
    return { grid: new LookupGridModel(next), issues: parsed.issues };
  }

  /** Empty and duplicate keys (line = grid row number). */
  issues(): ImportIssue[] {
    const seen = new Map<string, number>();
    const out: ImportIssue[] = [];
    this.rows.forEach((r, i) => {
      const key = r.key.trim();
      const first = seen.get(key);
      if (!key) out.push({ line: i + 1, message: 'The key is empty.' });
      else if (first) out.push({ line: i + 1, message: `Duplicate key "${key}" (first seen on line ${first}).` });
      else seen.set(key, i + 1);
    });
    return out;
  }

  toRows(): LookupRow[] {
    return this.rows.map((r) => ({ key: r.key.trim(), value: LookupGridModel.parseValue(r.value) }));
  }
}
