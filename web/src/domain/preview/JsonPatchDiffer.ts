import type { JsonArray, JsonObject, JsonValue } from '../json';
import { PatchPointer } from './PatchPointer';
import { PatchOpKind, type PatchOp } from './types';

/**
 * Structural RFC 6902 diff emitting add / remove / replace only, in the same order as the Go
 * `preview.StructuralDiffer` (sorted keys, removals first, array tails) so fixtures are shared.
 */
export class JsonPatchDiffer {
  static diff(prev: JsonValue, next: JsonValue): PatchOp[] {
    const ops: PatchOp[] = [];
    JsonPatchDiffer.walk(PatchPointer.root, prev, next, ops);
    return ops;
  }

  private static walk(path: string, a: JsonValue, b: JsonValue, ops: PatchOp[]): void {
    if (JsonPatchDiffer.isObject(a) && JsonPatchDiffer.isObject(b)) return JsonPatchDiffer.objects(path, a, b, ops);
    if (Array.isArray(a) && Array.isArray(b)) return JsonPatchDiffer.arrays(path, a, b, ops);
    if (!JsonPatchDiffer.sameScalar(a, b)) ops.push({ op: PatchOpKind.Replace, path, value: b });
  }

  private static objects(path: string, a: JsonObject, b: JsonObject, ops: PatchOp[]): void {
    for (const key of Object.keys(a).sort()) {
      if (!(key in b)) ops.push({ op: PatchOpKind.Remove, path: PatchPointer.child(path, key) });
    }
    for (const key of Object.keys(b).sort()) {
      const child = PatchPointer.child(path, key);
      if (key in a) JsonPatchDiffer.walk(child, a[key], b[key], ops);
      else ops.push({ op: PatchOpKind.Add, path: child, value: b[key] });
    }
  }

  private static arrays(path: string, a: JsonArray, b: JsonArray, ops: PatchOp[]): void {
    const common = Math.min(a.length, b.length);
    for (let i = 0; i < common; i++) JsonPatchDiffer.walk(PatchPointer.child(path, i), a[i], b[i], ops);
    for (let i = common; i < b.length; i++) ops.push({ op: PatchOpKind.Add, path: PatchPointer.child(path, i), value: b[i] });
    for (let i = a.length - 1; i >= b.length; i--) ops.push({ op: PatchOpKind.Remove, path: PatchPointer.child(path, i) });
  }

  private static sameScalar(a: JsonValue, b: JsonValue): boolean {
    if (typeof a === 'object' || typeof b === 'object') return a === b && a === null;
    return a === b;
  }

  private static isObject(v: JsonValue): v is JsonObject {
    return v !== null && typeof v === 'object' && !Array.isArray(v);
  }
}
