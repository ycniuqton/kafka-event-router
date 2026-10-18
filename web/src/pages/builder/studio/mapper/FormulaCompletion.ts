import { formulaFunctions } from '../../../../api/generated';
import { EventPath } from '../../../../domain/routeConfig/StepMapper';

/** One autocomplete choice of the formula bar. */
export type FormulaSuggestion = { label: string; insert: string; detail: string };

/** The token being typed and what can complete it. */
export type FormulaCompletionResult = { start: number; end: number; items: FormulaSuggestion[] };

/** Formula functions of the engine catalogue (`call` form + description). */
export type FormulaFunctionInfo = { name: string; call: string; description: string };

/** Autocomplete of the formula bar: field paths (`value.…`) and engine formula functions. */
export class FormulaCompletion {
  static readonly maxItems = 8;
  static readonly tokenPattern = /[A-Za-z0-9_.]+$/;
  static readonly fieldDetail = 'field';
  static readonly callSuffix = '(';

  static readonly functions: FormulaFunctionInfo[] = formulaFunctions.map((f) => ({ name: f.name, call: f.call, description: f.description }));

  /** Suggestions for the token ending at `caret` (nothing for an empty token). */
  static suggest(text: string, caret: number, fields: string[], functions: FormulaFunctionInfo[] = FormulaCompletion.functions): FormulaCompletionResult {
    const match = FormulaCompletion.tokenPattern.exec(text.slice(0, caret));
    const token = match ? match[0] : '';
    const start = caret - token.length;
    if (!token) return { start, end: caret, items: [] };
    const lower = token.toLowerCase();
    const fieldItems = fields
      .map((f) => EventPath.toDocument(f))
      .filter((p) => p.toLowerCase().startsWith(lower) && p !== token)
      .map((p) => ({ label: p, insert: p, detail: FormulaCompletion.fieldDetail }));
    const fnItems = functions
      .filter((f) => f.name.toLowerCase().startsWith(lower))
      .map((f) => ({ label: f.call, insert: f.name + FormulaCompletion.callSuffix, detail: f.description }));
    return { start, end: caret, items: [...fnItems, ...fieldItems].slice(0, FormulaCompletion.maxItems) };
  }

  /** Replaces the completed token with `insert`; returns the new text and caret. */
  static apply(text: string, result: FormulaCompletionResult, insert: string): { text: string; caret: number } {
    const next = text.slice(0, result.start) + insert + text.slice(result.end);
    return { text: next, caret: result.start + insert.length };
  }
}
