import type { Tone } from '../../../domain';

/** One series of a chart: values, legend label and colour tone (no tone = neutral surface). */
export type ChartSeries = { key: string; label: string; values: number[]; tone?: Tone };

/** Y-axis scaling for the overview charts (viewBox units: 0–100 high, 10 per point wide). */
export class ChartScale {
  static readonly height = 100;
  static readonly step = 10;
  static readonly niceSteps = [1, 2, 5, 10] as const;
  static readonly base = 10;

  /** Smallest 1/2/5×10ⁿ at or above the largest value (never below `floor`). */
  static niceMax(values: number[], floor = 1): number {
    const max = Math.max(floor, ...values);
    const magnitude = ChartScale.base ** Math.floor(Math.log10(max));
    const nice = ChartScale.niceSteps.find((s) => s * magnitude >= max) ?? ChartScale.base;
    return nice * magnitude;
  }

  static y(value: number, max: number): number {
    return ChartScale.height - (Math.min(value, max) / max) * ChartScale.height;
  }

  static points(values: number[], max: number): string {
    return values.map((v, i) => `${i * ChartScale.step},${ChartScale.y(v, max).toFixed(2)}`).join(' ');
  }
}
