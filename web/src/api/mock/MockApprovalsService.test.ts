import { describe, expect, it } from 'vitest';
import { ApprovalStatus } from '../../domain';
import { MockApprovalsService, MockApprovalsText } from './MockApprovalsService';
import { MockBackend } from './MockBackend';
import { MockTeamsService, MockTeamsText } from './MockTeamsService';

describe('MockApprovalsService', () => {
  it('approves once and refuses a second decision', async () => {
    const svc = new MockApprovalsService(new MockBackend());
    const [pending] = await svc.list({ status: ApprovalStatus.Pending });
    const approved = await svc.approve(pending, 'ok');
    expect(approved.status).toBe(ApprovalStatus.RollingOut);
    await expect(svc.reject(pending, 'late')).rejects.toThrow(MockApprovalsText.Decided);
  });

  it('needs a note to reject and a body to comment', async () => {
    const svc = new MockApprovalsService(new MockBackend());
    const [pending] = await svc.list({ status: ApprovalStatus.Pending });
    await expect(svc.reject(pending, ' ')).rejects.toThrow(MockApprovalsText.NoteRequired);
    await expect(svc.comment(pending.id, '')).rejects.toThrow(MockApprovalsText.InvalidComment);
    expect((await svc.comment(pending.id, 'why?')).body).toBe('why?');
    expect((await svc.get(pending.id)).comments).toHaveLength(2);
  });

  it('lists the caller own requests', async () => {
    const svc = new MockApprovalsService(new MockBackend());
    const [mine] = await svc.list({ mine: true });
    expect(mine.status).toBe(ApprovalStatus.Rejected);
  });
});

describe('MockTeamsService', () => {
  it('creates unique teams and guards grants', async () => {
    const svc = new MockTeamsService(new MockBackend());
    const team = await svc.create({ name: 'Risk', requireProdApproval: true });
    await expect(svc.create({ name: 'risk', requireProdApproval: false })).rejects.toThrow(MockTeamsText.NameTaken);
    const updated = await svc.update(team, { name: 'Risk & Fraud', requireProdApproval: false });
    expect(updated).toMatchObject({ name: 'Risk & Fraud', requireProdApproval: false, rowVersion: 2 });
    await expect(svc.update(team, { name: 'x', requireProdApproval: false })).rejects.toThrow(MockTeamsText.Stale);
    expect(await svc.grants('team-logistics')).toHaveLength(1);
  });
});
