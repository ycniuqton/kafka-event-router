/**
 * Reference instant of the mock dataset. Design labels such as "12 min ago" are stored
 * as ISO timestamps relative to this instant so `Format.relativeTime` reproduces them.
 */
export class MockTime {
  static readonly referenceIso = '2026-10-13T09:44:00Z';
  static readonly reference = Date.parse(MockTime.referenceIso);

  static readonly unitMs = {
    min: 60_000,
    h: 3_600_000,
    d: 86_400_000,
    w: 604_800_000,
  } as const;

  static ago(amount: number, unit: keyof typeof MockTime.unitMs): string {
    return new Date(MockTime.reference - amount * MockTime.unitMs[unit]).toISOString();
  }
}
