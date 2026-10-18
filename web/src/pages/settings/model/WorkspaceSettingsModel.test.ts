import { describe, expect, it } from 'vitest';
import { MockAccessService } from '../../../api/mock/MockAccessServices';
import { MockBackend } from '../../../api/mock/MockBackend';
import { AuditExportSink, Tone } from '../../../domain';
import { AuditExportView, WorkspaceText } from './WorkspaceSettingsModel';

const status = { enabled: true, sink: AuditExportSink.Webhook, target: 'https://siem/in', lagRows: 0, exportedTotal: 9, lastError: null, lastAttemptAt: null };

describe('Workspace section', () => {
  it('shows the export as off, healthy, catching up or failing', () => {
    expect(AuditExportView.of({ ...status, enabled: false, sink: AuditExportSink.None }).label).toBe(WorkspaceText.exportOff);
    expect(AuditExportView.of(status)).toMatchObject({ label: WorkspaceText.exportHealthy, tone: Tone.Ok });
    expect(AuditExportView.of({ ...status, lagRows: 4 }).tone).toBe(Tone.Warn);
    expect(AuditExportView.of({ ...status, lastError: 'webhook answered 503', lagRows: 4 })).toMatchObject({ label: WorkspaceText.exportFailing, tone: Tone.Err });
  });

  it('toggles AI assist with If-Match semantics (stale versions are rejected)', async () => {
    const svc = new MockAccessService(new MockBackend());
    const initial = await svc.settings();
    expect(initial.aiAssistEnabled).toBe(false);
    const on = await svc.updateSettings(initial, { aiAssistEnabled: true });
    expect(on).toMatchObject({ aiAssistEnabled: true, version: initial.version + 1 });
    await expect(svc.updateSettings(initial, { aiAssistEnabled: false })).rejects.toThrow(/Someone else changed this/);
    expect((await svc.updateSettings(on, { aiAssistEnabled: true })).version).toBe(on.version);
  });
});
