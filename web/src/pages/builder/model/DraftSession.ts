import type { Dispatch, SetStateAction } from 'react';
import {
  BranchSet,
  BuilderStep,
  FilterExpressionBuilder,
  FilterMode,
  type DraftDestination,
  type DraftSource,
  type FilterGroup,
  type RouteDraft,
  type TransformStep,
} from '../../../domain';

/** What can carry unsaved edits: one of the pipeline steps or the route name. */
export const DirtyKey = { ...BuilderStep, Name: 'name' } as const;
export type DirtyKey = (typeof DirtyKey)[keyof typeof DirtyKey];

/** The draft being edited plus which parts changed since the last save. */
export type DraftSessionState = { draft: RouteDraft; dirty: ReadonlySet<DirtyKey> };

type DraftUpdate = (draft: RouteDraft) => RouteDraft;

/** Pure state transitions of an editing session (design `setB`/`dirty`/`setFilter`/`setSteps`). */
export class DraftSession {
  static initial(draft: RouteDraft): DraftSessionState {
    return { draft, dirty: new Set() };
  }

  static apply(state: DraftSessionState, key: DirtyKey, update: DraftUpdate, change?: string): DraftSessionState {
    const draft = update(state.draft);
    const changes = change && !draft.changes.includes(change) ? [...draft.changes, change] : draft.changes;
    return { draft: { ...draft, changes }, dirty: new Set(state.dirty).add(key) };
  }

  static saved(state: DraftSessionState): DraftSessionState {
    return { draft: state.draft, dirty: new Set() };
  }

  /** Merges a filter patch and regenerates the expression while in visual mode. */
  static mergeFilter(filter: FilterGroup, patch: Partial<FilterGroup>): FilterGroup {
    const next = { ...filter, ...patch };
    return next.mode === FilterMode.Visual ? { ...next, expression: FilterExpressionBuilder.build(next) } : next;
  }
}

/** Editing commands of the builder; each one marks its step dirty and records a change label. */
export class DraftEditor {
  private readonly setState: Dispatch<SetStateAction<DraftSessionState>>;

  constructor(setState: Dispatch<SetStateAction<DraftSessionState>>) {
    this.setState = setState;
  }

  rename(name: string): void {
    this.commit(DirtyKey.Name, (d) => ({ ...d, name }));
  }

  patchSource(patch: Partial<DraftSource>, change?: string): void {
    this.commit(DirtyKey.Source, (d) => ({ ...d, source: { ...d.source, ...patch } }), change);
  }

  /** Source edits that are view choices (search, merge toggle) and do not count as unsaved changes. */
  adjustSource(patch: Partial<DraftSource>): void {
    this.setState((s) => ({ ...s, draft: { ...s.draft, source: { ...s.draft.source, ...patch } } }));
  }

  updateFilter(update: (filter: FilterGroup) => Partial<FilterGroup>, change?: string): void {
    this.commit(DirtyKey.Filter, (d) => ({ ...d, filter: DraftSession.mergeFilter(d.filter, update(d.filter)) }), change);
  }

  setSteps(steps: TransformStep[], change?: string): void {
    this.commit(DirtyKey.Transform, (d) => ({ ...d, steps }), change);
  }

  patchDestination(patch: Partial<DraftDestination>, change?: string): void {
    this.commit(DirtyKey.Destination, (d) => ({ ...d, destination: { ...d.destination, ...patch } }), change);
  }

  /** Edits another branch (P2); a view choice, not an unsaved change. */
  selectBranch(index: number): void {
    this.setState((s) => ({ ...s, draft: BranchSet.select(s.draft, index) }));
  }

  addBranch(): void {
    this.commit(DirtyKey.Destination, (d) => BranchSet.add(d), 'Branch added');
  }

  removeBranch(index: number): void {
    this.commit(DirtyKey.Destination, (d) => BranchSet.remove(d, index), 'Branch removed');
  }

  renameBranch(id: string): void {
    this.commit(DirtyKey.Destination, (d) => BranchSet.rename(d, id), 'Branch renamed');
  }

  /** Clears the unsaved marks; a first save also adopts the server's route id and status. */
  markSaved(saved?: Pick<RouteDraft, 'routeId' | 'status'>): void {
    this.setState((s) => DraftSession.saved(saved ? { ...s, draft: { ...s.draft, routeId: saved.routeId, status: saved.status } } : s));
  }

  private commit(key: DirtyKey, update: DraftUpdate, change?: string): void {
    this.setState((s) => DraftSession.apply(s, key, update, change));
  }
}
