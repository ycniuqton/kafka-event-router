import { describe, expect, it } from 'vitest';
import { DeadLetterKind, DeadLetterResolutionState, type DeadLetter } from '../../../domain';
import { DeadLetterCheckedRows, DeadLetterFacetOptions } from './DeadLetterBrowserModel';
import { BrowserLoad, DeadLetterBrowserState } from './DeadLetterBrowserState';
import { DeadLetterViewParams } from './DeadLetterQuery';

function row(id: string, routeId = 'r1'): DeadLetter {
  return { id, failedAt: '2026-10-13T09:41:07Z', kind: DeadLetterKind.TransformError, message: 'boom', stack: '', offset: 1, partition: 0, routeId, step: 'Step 1 · JavaScript', sampleId: -1 };
}

const page = (ids: string[], nextCursor: string | null = null) => ({ items: ids.map((id) => row(id)), nextCursor, truncated: false });

describe('DeadLetterBrowserState', () => {
  it('loads a first page, appends more without duplicates and selects the first row by default', () => {
    let s = DeadLetterBrowserState.initial().loaded(page(['a', 'b'], 'c1'), false);
    expect(s.hasMore).toBe(true);
    expect(s.selected?.id).toBe('a');
    s = s.loadingMore();
    expect(s.load).toBe(BrowserLoad.More);
    s = s.loaded(page(['b', 'c']), true);
    expect(s.items.map((d) => d.id)).toEqual(['a', 'b', 'c']);
    expect(s.hasMore).toBe(false);
    expect(s.loading).toBe(false);
  });

  it('toggles checks, select-all and drops resolved rows moving the selection forward', () => {
    let s = DeadLetterBrowserState.initial('b').loaded(page(['a', 'b', 'c']), false);
    expect(s.selected?.id).toBe('b');
    s = s.toggle('a').toggle('b');
    expect(s.checkedRows.map((d) => d.id)).toEqual(['a', 'b']);
    s = s.toggleAll();
    expect(s.allChecked).toBe(true);
    s = s.without(['b']);
    expect(s.items.map((d) => d.id)).toEqual(['a', 'c']);
    expect(s.selected?.id).toBe('c');
    expect(s.checked).toEqual(['a', 'c']);
    expect(s.toggleAll().checked).toEqual([]);
  });

  it('keeps failed retries with their new error and resets on reload / failure', () => {
    let s = DeadLetterBrowserState.initial().loaded(page(['a', 'b']), false).withFailures([{ id: 'b', message: 'still broken' }]);
    expect(s.items[1].message).toBe('still broken');
    s = s.reloading();
    expect(s.items).toEqual([]);
    expect(s.loading).toBe(true);
    s = s.failed('upstream down');
    expect(s).toMatchObject({ load: BrowserLoad.Failed, error: 'upstream down' });
  });
});

describe('DeadLetterViewParams', () => {
  it('parses URL filters, ignores unknown kinds and builds the server filter', () => {
    const params = new URLSearchParams({ route: 'r9', type: `${DeadLetterKind.Timeout},bogus`, range: '1h' });
    const view = DeadLetterViewParams.parse(params);
    expect(view).toMatchObject({ routeId: 'r9', kinds: [DeadLetterKind.Timeout], range: '1h', includeResolved: false });
    expect(DeadLetterViewParams.parse(params, 'fixed').routeId).toBe('fixed');
    const filter = DeadLetterViewParams.filter(view, new Date('2026-10-13T10:00:00Z'));
    expect(filter).toMatchObject({ routeId: 'r9', kinds: [DeadLetterKind.Timeout], since: '2026-10-13T09:00:00.000Z' });
    expect(DeadLetterViewParams.toggleKind(view, DeadLetterKind.Timeout)).toBeNull();
    expect(DeadLetterViewParams.isFiltered(DeadLetterViewParams.parse(new URLSearchParams()), undefined)).toBe(false);
  });
});

describe('DeadLetterViewParams P2 filters', () => {
  it('parses step, version, text and show-resolved and clears them', () => {
    const params = new URLSearchParams({ step: 'step-0', ver: '3', q: ' reduce ', resolved: '1' });
    const view = DeadLetterViewParams.parse(params, 'r1');
    expect(view).toMatchObject({ routeId: 'r1', stepId: 'step-0', versionNo: 3, text: 'reduce', includeResolved: true });
    expect(DeadLetterViewParams.filter(view, new Date())).toMatchObject({ stepId: 'step-0', versionNo: 3, text: 'reduce', includeResolved: true });
    expect(DeadLetterViewParams.isFiltered(view, 'r1')).toBe(true);
    expect(DeadLetterViewParams.parse(new URLSearchParams({ ver: 'x' })).versionNo).toBeUndefined();
    expect(Object.keys(DeadLetterViewParams.cleared('r1'))).not.toContain('route');
    expect(DeadLetterViewParams.key(view)).not.toBe(DeadLetterViewParams.key({ ...view, includeResolved: false }));
  });

  it('derives step / version options and checked-row actions', () => {
    const rows = [
      { ...row('a'), stepId: 'step-0', versionNo: 2 },
      { ...row('b'), stepId: 'step-1', step: 'Step 2 · Mask', versionNo: 3, resolution: { state: DeadLetterResolutionState.Discarded, by: 'u', at: '' } },
    ];
    expect(DeadLetterFacetOptions.steps(rows).map((s) => s.id)).toEqual(['step-0', 'step-1']);
    expect(DeadLetterFacetOptions.versions(rows, 7)).toEqual([7, 3, 2]);
    expect(DeadLetterCheckedRows.discarded(rows)).toEqual(['b']);
    expect(DeadLetterCheckedRows.unresolved(rows)).toEqual(['a']);
  });
});
