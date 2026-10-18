import { JsonPatchApplier, PatchPointer, type JsonValue, type PatchOp } from '../../../../domain';

/**
 * Rebuilds per-step snapshots from preview step patches (live-preview `JsonPatchApplier`). A patch
 * addresses the single event (`/value/x`) while one event goes in and out, and the event list
 * (`/0`, `/1/value`) when a step drops the event or emits several.
 */
export class PatchReplayer {
  static readonly indexToken = /^\d+$/;

  /** Event lists after each step; a patch that no longer applies ends the replay with an empty list. */
  static replay(input: JsonValue, patches: readonly (readonly PatchOp[])[]): JsonValue[][] {
    const out: JsonValue[][] = [];
    let current: JsonValue[] = [input];
    for (const patch of patches) {
      current = PatchReplayer.step(current, patch);
      out.push(current);
    }
    return out;
  }

  static step(events: JsonValue[], patch: readonly PatchOp[]): JsonValue[] {
    if (!patch.length) return events;
    try {
      if (PatchReplayer.addressesList(events, patch)) return JsonPatchApplier.apply(events, patch) as JsonValue[];
      return [JsonPatchApplier.apply(events[0], patch)];
    } catch {
      return [];
    }
  }

  private static addressesList(events: JsonValue[], patch: readonly PatchOp[]): boolean {
    if (events.length !== 1) return true;
    return patch.every((op) => PatchReplayer.indexToken.test(PatchPointer.parse(op.path)[0] ?? ''));
  }
}
