import { describe, expect, it } from 'vitest';
import { ImportAction, RouteStatus, TransformMode, type ImportReport, type Route } from '../../../domain';
import { ImportReportModel } from './ImportReportModel';
import { AnyOption, RouteFacets, RouteListQuery, StatusFilterAll, StatusFilterOptions } from './RouteListModel';

const route = (id: string, owner: string, status: RouteStatus): Route => ({
  id,
  name: `route-${id}`,
  status,
  owner,
  source: { connectionId: 'c1', topics: ['t'] },
  destination: { connectionId: 'c2', topic: 'o' },
  filterSummary: 'all events',
  transformMode: TransformMode.None,
  metrics: { eventsPerSec: 0, matchPct: 0, lag: 0 },
  updatedAt: '2026-10-14T09:00:00Z',
  version: 1,
});

const routes = [route('a', 'payments', RouteStatus.Running), route('b', 'partners', RouteStatus.Paused), route('c', 'payments', RouteStatus.Archived)];
const filter = { query: '', status: StatusFilterAll, connectionId: AnyOption, tag: AnyOption, owner: AnyOption } as const;

describe('routes list (P2: owner filter, archived chip)', () => {
  it('filters by owner team and lists owners', () => {
    expect(RouteListQuery.apply(routes, { ...filter, owner: 'payments' }).map((r) => r.id)).toEqual(['a']);
    expect(RouteFacets.owners(routes)).toEqual(['partners', 'payments']);
  });

  it('keeps archived routes out of "All" and counts them only on their chip', () => {
    expect(RouteListQuery.apply(routes, filter).map((r) => r.id)).toEqual(['a', 'b']);
    expect(RouteListQuery.apply(routes, { ...filter, status: RouteStatus.Archived }).map((r) => r.id)).toEqual(['c']);
    expect(StatusFilterOptions.count(routes, StatusFilterAll)).toBe(2);
    expect(StatusFilterOptions.count(routes, RouteStatus.Archived)).toBe(1);
    expect(StatusFilterOptions.parse('archived')).toBe(RouteStatus.Archived);
  });
});

describe('ImportReportModel', () => {
  const report: ImportReport = {
    dryRun: true,
    items: [
      { index: 0, name: 'a', routeId: null, action: ImportAction.Created, issues: [] },
      { index: 1, name: 'b', routeId: 'x', action: ImportAction.SkippedExists, issues: [] },
      { index: 2, name: 'c', routeId: null, action: ImportAction.Created, issues: [] },
    ],
  };

  it('summarises planned outcomes and tells whether a real run writes', () => {
    expect(ImportReportModel.summary(report)).toBe('2 × will be created · 1 × skipped · name exists');
    expect(ImportReportModel.writes(report)).toBe(true);
    expect(ImportReportModel.writes({ dryRun: true, items: [report.items[1]] })).toBe(false);
    expect(ImportReportModel.label(report.items[0], false)).toBe('Created as draft');
  });
});
