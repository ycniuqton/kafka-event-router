/** Number inputs: typed text → number (empty or invalid → NaN so validation flags it). */
export class NumberText {
  static of(text: string): number {
    return text.trim() === '' ? Number.NaN : Number(text);
  }

  /** Display value of a number field (NaN shows as empty). */
  static show(value: number): string | number {
    return Number.isNaN(value) ? '' : value;
  }
}
