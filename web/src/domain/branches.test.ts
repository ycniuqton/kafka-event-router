import { describe, expect, it } from 'vitest';
import type { RouteConfig } from '../api/generated';
import { BranchSet } from './branches';
import { RouteStatus } from './route';
import { RouteDraftMapper } from './routeConfig/RouteDraftMapper';

const doc: RouteConfig = {
  apiVersion: 'router/v1',
  name: 'split',
  source: { connectionId: 'c1', topics: ['orders'], format: { type: 'json' } },
  branches: [
    { id: 'eu', filter: { type: 'cond', field: 'value.region', op: 'eq', value: 'EU' }, destination: { connectionId: 'c2', topic: 'orders-eu' } },
    {
      id: 'rest',
      filter: { type: 'not', child: { type: 'cond', field: 'value.region', op: 'eq', value: 'EU' } },
      destination: { connectionId: 'c1', topic: 'orders-rest' },
    },
  ],
  errorPolicy: { onError: 'dlq' },
};
const meta = { routeId: 'r1', name: 'split', status: RouteStatus.Running };

describe('BranchSet (routes#p2-fe-branches)', () => {
  it('loads every branch, editing the first and parking the others', () => {
    const { draft } = RouteDraftMapper.fromDocument(doc, meta);
    expect(BranchSet.all(draft).map((b) => b.id)).toEqual(['eu', 'rest']);
    expect(draft.destination.topic).toBe('orders-eu');
    expect(draft.filter.rows[0].value).toBe('EU');
  });

  it('switches branches without losing edits and keeps document order', () => {
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, meta);
    const edited = { ...draft, destination: { ...draft.destination, topic: 'orders-eu-v2' } };
    const rest = BranchSet.select(edited, 1);
    expect(rest.destination.topic).toBe('orders-rest');
    expect(BranchSet.state(rest)).toMatchObject({ index: 1, id: 'rest' });
    const out = RouteDraftMapper.toDocument(rest, snapshot);
    expect(out.branches.map((b) => [b.id, b.destination.topic])).toEqual([
      ['eu', 'orders-eu-v2'],
      ['rest', 'orders-rest'],
    ]);
    expect(out.branches[1].filter).toEqual(doc.branches[1].filter);
  });

  it('adds, renames and removes branches', () => {
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, meta);
    const added = BranchSet.add(draft);
    expect(BranchSet.count(added)).toBe(3);
    expect(BranchSet.state(added).id).toBe('branch-3');
    expect(added.destination.connectionId).toBe('c2');
    const renamed = BranchSet.rename({ ...added, destination: { ...added.destination, topic: 'audit' } }, 'audit');
    expect(BranchSet.rename(renamed, 'eu')).toBe(renamed);
    expect(BranchSet.rename(renamed, 'bad id!')).toBe(renamed);
    const out = RouteDraftMapper.toDocument(renamed, snapshot);
    expect(out.branches.map((b) => b.id)).toEqual(['eu', 'rest', 'audit']);
    expect(out.branches[2].filter).toBeUndefined();
    const removed = BranchSet.remove(renamed, 0);
    expect(BranchSet.all(removed).map((b) => b.id)).toEqual(['rest', 'audit']);
    expect(removed.destination.onError).toBe(draft.destination.onError);
  });

  it('a single main branch has no branching state', () => {
    const one: RouteConfig = { ...doc, branches: [{ id: 'main', destination: { connectionId: 'c1', topic: 'o' } }] };
    const { draft } = RouteDraftMapper.fromDocument(one, meta);
    expect(draft.branching).toBeUndefined();
    expect(BranchSet.remove(draft, 0)).toBe(draft);
  });
});
