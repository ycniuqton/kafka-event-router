/** Auto-retry of transient failures (route document `errorPolicy.retry`), with server defaults filled in. */
export type RetryPolicySettings = { enabled: boolean; maxAttempts: number; baseDelayMs: number; maxDelayMs: number };

/** Bounds, defaults and the backoff schedule of the retry policy (same as the route schema). */
export class RetryPolicyRules {
  static readonly attempts = { min: 1, max: 10, default: 5 } as const;
  static readonly baseDelay = { min: 100, max: 3_600_000, default: 1000 } as const;
  static readonly maxDelay = { min: 100, max: 86_400_000, default: 300_000 } as const;
  static readonly factor = 2;
  static readonly msPerSecond = 1000;
  static readonly secondsPerMinute = 60;
  static readonly minutesPerHour = 60;
  static readonly arrow = ' → ';

  static readonly defaults: RetryPolicySettings = {
    enabled: false,
    maxAttempts: RetryPolicyRules.attempts.default,
    baseDelayMs: RetryPolicyRules.baseDelay.default,
    maxDelayMs: RetryPolicyRules.maxDelay.default,
  };

  /** delay(n) = min(base · 2^(n−1), max) for n = 1..maxAttempts. */
  static delays(p: RetryPolicySettings): number[] {
    return Array.from({ length: p.maxAttempts }, (_, i) => Math.min(p.baseDelayMs * RetryPolicyRules.factor ** i, p.maxDelayMs));
  }

  /** "1 s → 2 s → 4 s" */
  static schedule(p: RetryPolicySettings): string {
    return RetryPolicyRules.delays(p).map((d) => RetryPolicyRules.duration(d)).join(RetryPolicyRules.arrow);
  }

  static duration(ms: number): string {
    const s = ms / RetryPolicyRules.msPerSecond;
    if (s < RetryPolicyRules.secondsPerMinute) return `${Number(s.toFixed(1))} s`;
    const m = s / RetryPolicyRules.secondsPerMinute;
    if (m < RetryPolicyRules.minutesPerHour) return `${Number(m.toFixed(1))} min`;
    return `${Number((m / RetryPolicyRules.minutesPerHour).toFixed(1))} h`;
  }

  /** Field → message for a policy being edited (only checked when enabled). */
  static validate(p: RetryPolicySettings): Partial<Record<keyof RetryPolicySettings, string>> {
    const out: Partial<Record<keyof RetryPolicySettings, string>> = {};
    if (!p.enabled) return out;
    const within = (v: number, b: { min: number; max: number }) => Number.isInteger(v) && v >= b.min && v <= b.max;
    if (!within(p.maxAttempts, RetryPolicyRules.attempts)) out.maxAttempts = `Use ${RetryPolicyRules.attempts.min}–${RetryPolicyRules.attempts.max} attempts.`;
    if (!within(p.baseDelayMs, RetryPolicyRules.baseDelay)) out.baseDelayMs = `Use ${RetryPolicyRules.baseDelay.min}–${RetryPolicyRules.baseDelay.max} ms.`;
    if (!within(p.maxDelayMs, RetryPolicyRules.maxDelay)) out.maxDelayMs = `Use ${RetryPolicyRules.maxDelay.min}–${RetryPolicyRules.maxDelay.max} ms.`;
    else if (p.maxDelayMs < p.baseDelayMs) out.maxDelayMs = 'The max delay must be at least the base delay.';
    return out;
  }
}
