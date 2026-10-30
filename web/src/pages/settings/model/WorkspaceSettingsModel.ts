import { AuditExportSink, Tone, type AuditExportStatus } from '../../../domain';

/** Choice of the AI-assist control. */
export const AssistChoice = { On: 'on', Off: 'off' } as const;
export type AssistChoice = (typeof AssistChoice)[keyof typeof AssistChoice];

/** Texts of the workspace section. */
export const WorkspaceText = {
  assistHint: 'Transform Studio suggestions and explanations (off by default)',
  routerAlertHint: 'Alert on router-caused dead letters for every route (on by default; each route can still disable its rule)',
  assistOn: 'On',
  assistOff: 'Off',
  saved: 'Workspace settings saved',
  exportOff: 'Not configured',
  exportOffHint: 'Set ROUTER_AUDIT_EXPORT_SINK (webhook or syslog) to stream the audit log to your SIEM.',
  exportFailing: 'Failing',
  exportCatchingUp: 'Catching up',
  exportHealthy: 'Healthy',
} as const;

/** The SIEM export status as one badge plus a detail line. */
export type ExportStatusView = { label: string; tone: Tone; detail: string };

/** Presents the continuous audit export status (Settings → Workspace). */
export class AuditExportView {
  private static readonly sinkLabel: Record<AuditExportSink, string> = {
    [AuditExportSink.None]: 'None',
    [AuditExportSink.Webhook]: 'Webhook',
    [AuditExportSink.Syslog]: 'Syslog',
  };

  static of(status: AuditExportStatus): ExportStatusView {
    if (!status.enabled) return { label: WorkspaceText.exportOff, tone: Tone.Off, detail: WorkspaceText.exportOffHint };
    const sink = `${AuditExportView.sinkLabel[status.sink]} · ${status.target}`;
    if (status.lastError) return { label: WorkspaceText.exportFailing, tone: Tone.Err, detail: `${sink} · ${status.lastError} · ${status.lagRows} entries waiting` };
    const tone = status.lagRows > 0 ? Tone.Warn : Tone.Ok;
    return { label: status.lagRows > 0 ? WorkspaceText.exportCatchingUp : WorkspaceText.exportHealthy, tone, detail: `${sink} · ${status.exportedTotal} exported · ${status.lagRows} waiting` };
  }
}
