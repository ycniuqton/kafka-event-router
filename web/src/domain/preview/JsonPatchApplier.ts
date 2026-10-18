import type { JsonArray, JsonObject, JsonValue } from '../json';
import { PatchPointer } from './PatchPointer';
import { PatchOpKind, type PatchOp } from './types';

/** Container kinds a patch token can address. */
type Container = JsonObject | JsonArray;

/** Applies preview patches (add / remove / replace) to a copy of a document. */
export class JsonPatchApplier {
  static readonly appendToken = '-';

  static apply(doc: JsonValue, ops: readonly PatchOp[]): JsonValue {
    let out: JsonValue = structuredClone(doc);
    for (const op of ops) out = JsonPatchApplier.applyOne(out, op);
    return out;
  }

  /** Applies the patches of several consecutive steps in order. */
  static applyAll(doc: JsonValue, steps: readonly (readonly PatchOp[])[]): JsonValue {
    return steps.reduce<JsonValue>((acc, ops) => JsonPatchApplier.apply(acc, ops), doc);
  }

  private static applyOne(doc: JsonValue, op: PatchOp): JsonValue {
    const tokens = PatchPointer.parse(op.path);
    if (tokens.length === 0) return op.op === PatchOpKind.Remove ? null : (op.value ?? null);
    const last = tokens.pop() as string;
    const parent = JsonPatchApplier.resolve(doc, tokens);
    if (Array.isArray(parent)) JsonPatchApplier.inArray(parent, last, op);
    else JsonPatchApplier.inObject(parent, last, op);
    return doc;
  }

  private static resolve(doc: JsonValue, tokens: string[]): Container {
    let node: JsonValue = doc;
    for (const token of tokens) {
      const next: JsonValue | undefined = Array.isArray(node) ? node[Number(token)] : (node as JsonObject | null)?.[token];
      if (next === null || typeof next !== 'object') throw new Error(`patch path not found: ${token}`);
      node = next;
    }
    if (node === null || typeof node !== 'object') throw new Error('patch target is not a container');
    return node;
  }

  private static inObject(obj: JsonObject, key: string, op: PatchOp): void {
    if (op.op === PatchOpKind.Remove) delete obj[key];
    else obj[key] = op.value ?? null;
  }

  private static inArray(list: JsonArray, token: string, op: PatchOp): void {
    const index = token === JsonPatchApplier.appendToken ? list.length : Number(token);
    if (op.op === PatchOpKind.Remove) list.splice(index, 1);
    else if (op.op === PatchOpKind.Add) list.splice(index, 0, op.value ?? null);
    else list[index] = op.value ?? null;
  }
}
