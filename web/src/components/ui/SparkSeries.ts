/** Deterministic pseudo-random bar heights (design `spark(seed, flat)`), in percent. */
export class SparkSeries {
  static readonly bars = 18;
  static readonly flatHeight = 6;

  static readonly minHeight = 4;
  static readonly full = 100;

  /** Real series scaled to percent of its peak (last `bars` points; an all-zero series renders flat). */
  static fromValues(values: number[], bars = SparkSeries.bars): number[] {
    const tail = values.slice(-bars);
    const peak = Math.max(0, ...tail);
    if (peak <= 0) return tail.map(() => SparkSeries.flatHeight);
    return tail.map((v) => Math.max(SparkSeries.minHeight, Math.round((v / peak) * SparkSeries.full)));
  }

  static heights(seed: number, flat = false, bars = SparkSeries.bars): number[] {
    return Array.from({ length: bars }, (_, i) => (flat ? SparkSeries.flatHeight : SparkSeries.height(seed, i)));
  }

  private static height(seed: number, i: number): number {
    const wave = 45 * Math.abs(Math.sin(seed * 1.7 + i * 0.55));
    const step = (20 * ((i * seed) % 3)) / 3;
    return Math.round(Math.min(100, 25 + wave + step));
  }
}
