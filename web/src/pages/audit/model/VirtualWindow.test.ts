import { describe, expect, it } from 'vitest';
import { AnyAuditQuery } from '../../../domain';
import { MockAuditService } from '../../../api/mock/MockAuditService';
import { MockBackend } from '../../../api/mock/MockBackend';
import { AuditLayout } from './AuditFilter';
import { VirtualWindow } from './VirtualWindow';

describe('AuditTable virtual window', () => {
  const win = new VirtualWindow(40, 5);

  it('renders only the visible rows plus overscan and keeps the scroll height', () => {
    const slice = win.slice(10_000, 4_000, 400);
    expect(slice).toEqual({ start: 95, end: 115, padTop: 3_800, padBottom: (10_000 - 115) * 40 });
    expect(slice.end - slice.start).toBeLessThan(30);
    expect(win.slice(3, 0, 400)).toEqual({ start: 0, end: 3, padTop: 0, padBottom: 0 });
  });

  it('asks for the next page near the end of the loaded rows', () => {
    expect(win.nearEnd(100, 0, 400, AuditLayout.loadAheadRows)).toBe(false);
    expect(win.nearEnd(100, 3_200, 400, AuditLayout.loadAheadRows)).toBe(true);
    expect(win.nearEnd(5, 0, 400, AuditLayout.loadAheadRows)).toBe(true);
  });
});

describe('AuditTable pagination', () => {
  it('appends cursor pages until the log is exhausted, without duplicates', async () => {
    const svc = new MockAuditService(new MockBackend());
    const ids: string[] = [];
    let cursor: string | null = null;
    let pages = 0;
    do {
      const page = await svc.query(AnyAuditQuery, cursor);
      ids.push(...page.items.map((e) => e.id));
      cursor = page.nextCursor;
      pages++;
    } while (cursor && pages < 10);
    expect(pages).toBe(2);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toHaveLength(7);
  });
});
