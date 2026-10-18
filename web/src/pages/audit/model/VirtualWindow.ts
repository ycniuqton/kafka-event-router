/** The rows to render for a scroll position: `[start, end)` plus the spacer heights above and below. */
export type VirtualSlice = { start: number; end: number; padTop: number; padBottom: number };

/** Fixed-height row windowing for long lists (the audit table renders only what is on screen plus overscan). */
export class VirtualWindow {
  private readonly rowHeight: number;
  private readonly overscan: number;

  constructor(rowHeight: number, overscan: number) {
    this.rowHeight = rowHeight;
    this.overscan = overscan;
  }

  slice(count: number, scrollTop: number, viewportHeight: number): VirtualSlice {
    const first = Math.floor(Math.max(0, scrollTop) / this.rowHeight);
    const visible = Math.ceil(viewportHeight / this.rowHeight);
    const start = Math.max(0, first - this.overscan);
    const end = Math.min(count, first + visible + this.overscan);
    return { start, end, padTop: start * this.rowHeight, padBottom: Math.max(0, count - end) * this.rowHeight };
  }

  /** Whether the viewport is within `aheadRows` of the last loaded row (time to fetch the next page). */
  nearEnd(count: number, scrollTop: number, viewportHeight: number, aheadRows: number): boolean {
    return (scrollTop + viewportHeight) / this.rowHeight >= count - aheadRows;
  }
}
