/** Reads and writes dotted paths (`value.items.0.sku`) inside plain JSON objects. */
export class FieldPath {
  static readonly separator = '.';

  static get(target: unknown, path: string): unknown {
    return path
      .split(FieldPath.separator)
      .reduce<unknown>((node, key) => (node == null ? undefined : (node as Record<string, unknown>)[key]), target);
  }

  /** Writes a value, creating intermediate objects when missing. */
  static set(target: Record<string, unknown>, path: string, value: unknown): void {
    const keys = path.split(FieldPath.separator);
    const last = keys.pop() as string;
    let node = target;
    for (const key of keys) {
      if (node[key] == null || typeof node[key] !== 'object') node[key] = {};
      node = node[key] as Record<string, unknown>;
    }
    node[last] = value;
  }

  static remove(target: Record<string, unknown>, path: string): void {
    const keys = path.split(FieldPath.separator);
    const last = keys.pop() as string;
    const parent = keys.length ? FieldPath.get(target, keys.join(FieldPath.separator)) : target;
    if (parent && typeof parent === 'object') delete (parent as Record<string, unknown>)[last];
  }

  /** Path without the leading `value.` segment, as shown in summaries. */
  static short(path: string): string {
    return path.replace('value.', '');
  }
}
