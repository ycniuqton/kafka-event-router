/** A connector between an input field row and an output field row. */
export type ConnectorLink = { from: string; to: string };

/** Straight-ish connector geometry in the container's coordinates. */
export type ConnectorLine = { x1: number; y1: number; x2: number; y2: number };

/** Keys of connector endpoints: input rows by field path, output rows by entry index. */
export class ConnectorKey {
  static input(path: string): string {
    return `in:${path}`;
  }

  static output(index: number): string {
    return `out:${index}`;
  }
}

/** DOM rows of the mapper by connector key; measures the line between two rows. */
export class ConnectorRegistry {
  private readonly nodes = new Map<string, HTMLElement>();

  /** Ref callback registering (or, with null, forgetting) the row of `key`. */
  ref(key: string): (el: HTMLElement | null) => void {
    return (el) => {
      if (el) this.nodes.set(key, el);
      else this.nodes.delete(key);
    };
  }

  /** From the input row's right edge to the output row's left edge; null when a row is not rendered. */
  line(link: ConnectorLink, container: HTMLElement): ConnectorLine | null {
    const from = this.nodes.get(link.from);
    const to = this.nodes.get(link.to);
    if (!from || !to) return null;
    const box = container.getBoundingClientRect();
    const a = from.getBoundingClientRect();
    const b = to.getBoundingClientRect();
    return { x1: a.right - box.left, y1: a.top + a.height / 2 - box.top, x2: b.left - box.left, y2: b.top + b.height / 2 - box.top };
  }
}
