/** Orders two picked version numbers as (older, newer). */
export class VersionCompare {
  static ordered(a: number, b: number): [number, number] {
    return a < b ? [a, b] : [b, a];
  }
}

/** Up to two versions ticked for comparison; ticking a third replaces the oldest pick. */
export class CompareSelection {
  static readonly limit = 2;

  static toggle(picked: number[], version: number): number[] {
    if (picked.includes(version)) return picked.filter((v) => v !== version);
    return [...picked, version].slice(-CompareSelection.limit);
  }
}
