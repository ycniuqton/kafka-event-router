import { describe, expect, it } from 'vitest';
import { AnyAuditQuery, AuditActions, AuditFileFormat } from '../../domain';
import { MockAuditFile, MockAuditService } from './MockAuditService';
import { MockBackend } from './MockBackend';

describe('MockAuditService', () => {
  it('filters like the server and pages with a cursor', async () => {
    const svc = new MockAuditService(new MockBackend());
    const first = await svc.query(AnyAuditQuery, null);
    expect(first.items).toHaveLength(5);
    const second = await svc.query(AnyAuditQuery, first.nextCursor);
    expect(second.nextCursor).toBeNull();
    const raw = await svc.query({ ...AnyAuditQuery, text: 'unmasked', actorId: 'u-quincy', sensitiveOnly: true }, null);
    expect(raw.items.map((e) => e.id)).toEqual(['au1']);
    expect((await svc.actors()).map((a) => a.name)).toContain('Quincy N.');
  });

  it('applies action, object type, team and time range filters', async () => {
    const svc = new MockAuditService(new MockBackend());
    const ids = async (q: Partial<typeof AnyAuditQuery>) => (await svc.query({ ...AnyAuditQuery, ...q }, null)).items.map((e) => e.id);
    expect(await ids({ actions: ['route.paused', 'route.created'] })).toEqual(['au4', 'au7']);
    expect(await ids({ objectType: 'route', teamId: 'team-logistics' })).toEqual(['au4', 'au6']);
    expect(await ids({ from: '2026-10-13T09:00:00Z', to: '2026-10-13T09:40:00Z' })).toEqual(['au2', 'au3']);
    const actions = await svc.actions();
    expect(actions.find((a) => a.action === 'sample.viewed_raw')?.sensitive).toBe(true);
  });

  it('renders export files with escaped CSV cells', () => {
    const csv = MockAuditFile.render([{ ...firstEntry(), object: 'a, "b"' }], AuditFileFormat.Csv);
    expect(csv.split('\n')[0]).toContain('id,at,actor_id');
    expect(csv).toContain('"a, ""b"""');
    expect(MockAuditFile.render([firstEntry()], AuditFileFormat.Jsonl).trim().split('\n')).toHaveLength(1);
  });

  it('labels known actions and humanises unknown ones', () => {
    expect(AuditActions.label('sample.viewed_raw')).toBe('Viewed raw (unmasked) samples');
    expect(AuditActions.label('route.replayed')).toBe('Route replayed');
  });
});

function firstEntry() {
  return new MockBackend().db.audit[0];
}
