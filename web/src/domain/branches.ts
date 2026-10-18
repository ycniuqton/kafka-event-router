import { FilterCombinator, FilterMode } from './filter';
import type { BranchState, DraftBranch, RouteDraft } from './builder';

/**
 * Branches of a builder draft (spec §7 "+ Add branch", P2). The edited branch is kept in the draft's
 * filter / steps / destination so every step editor works unchanged; switching swaps it with another one.
 */
export class BranchSet {
  static readonly mainId = 'main';
  static readonly idPrefix = 'branch-';
  static readonly maxBranches = 16;
  private static readonly idPattern = /^[A-Za-z0-9_-]{1,64}$/;

  static state(draft: RouteDraft): BranchState {
    return draft.branching ?? { index: 0, id: BranchSet.mainId, others: [] };
  }

  /** Every branch in document order (the edited one built from the draft fields). */
  static all(draft: RouteDraft): DraftBranch[] {
    const state = BranchSet.state(draft);
    const current: DraftBranch = { id: state.id, filter: draft.filter, steps: draft.steps, destination: draft.destination };
    const list = state.others.slice();
    list.splice(state.index, 0, current);
    return list;
  }

  /** The edited branch for preview lookups: its position, and its id when the route has several branches. */
  static at(draft: RouteDraft): { index: number; id: string | null } {
    const state = BranchSet.state(draft);
    return { index: state.index, id: BranchSet.count(draft) > 1 ? state.id : null };
  }

  static count(draft: RouteDraft): number {
    return BranchSet.state(draft).others.length + 1;
  }

  /** Edits branch `index` next; the route-level on-error choice stays with the draft. */
  static select(draft: RouteDraft, index: number): RouteDraft {
    const list = BranchSet.all(draft);
    if (index < 0 || index >= list.length) return draft;
    return BranchSet.focus(draft, list, index);
  }

  /** Appends a branch writing to the edited branch's connection and selects it. */
  static add(draft: RouteDraft): RouteDraft {
    const list = BranchSet.all(draft);
    if (list.length >= BranchSet.maxBranches) return draft;
    const fresh: DraftBranch = {
      id: BranchSet.nextId(list),
      filter: { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '', groups: [] },
      steps: [],
      destination: { ...draft.destination, topic: '', createTopic: null, schemaSubject: '', addHeaders: [], keepHeaders: [] },
    };
    return BranchSet.focus(draft, [...list, fresh], list.length);
  }

  /** Removes branch `index` (never the last one) and edits its neighbour. */
  static remove(draft: RouteDraft, index: number): RouteDraft {
    const list = BranchSet.all(draft);
    if (list.length <= 1 || index < 0 || index >= list.length) return draft;
    const rest = list.filter((_, i) => i !== index);
    return BranchSet.focus(draft, rest, Math.min(index, rest.length - 1));
  }

  /** Renames the edited branch when the id is valid and unused. */
  static rename(draft: RouteDraft, id: string): RouteDraft {
    const state = BranchSet.state(draft);
    if (!BranchSet.idPattern.test(id) || state.others.some((b) => b.id === id)) return draft;
    return { ...draft, branching: { ...state, id } };
  }

  static isValidId(id: string): boolean {
    return BranchSet.idPattern.test(id);
  }

  private static focus(draft: RouteDraft, list: DraftBranch[], index: number): RouteDraft {
    const target = list[index];
    const onError = draft.destination.onError;
    const others = list.filter((_, i) => i !== index);
    const branching = list.length > 1 || target.id !== BranchSet.mainId ? { index, id: target.id, others } : undefined;
    return { ...draft, filter: target.filter, steps: target.steps, destination: { ...target.destination, onError }, branching };
  }

  private static nextId(list: DraftBranch[]): string {
    const used = new Set(list.map((b) => b.id));
    let n = list.length + 1;
    while (used.has(`${BranchSet.idPrefix}${n}`)) n++;
    return `${BranchSet.idPrefix}${n}`;
  }
}
