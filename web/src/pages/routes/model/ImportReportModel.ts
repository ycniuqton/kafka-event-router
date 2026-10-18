import { ImportAction, ImportMode, IssueSeverity, Tone, type ImportItem, type ImportReport } from '../../../domain';

/** Labels and tones of import outcomes, plus the summary line of a report (dry run or real). */
export class ImportReportModel {
  static readonly modeOptions = [
    { value: ImportMode.Create, label: 'New routes only' },
    { value: ImportMode.Upsert, label: 'Also update drafts' },
  ] as const;

  private static readonly labels: Record<ImportAction, { done: string; planned: string; tone: Tone }> = {
    [ImportAction.Created]: {
      done: 'Created as draft',
      planned: 'Will be created',
      tone: Tone.Ok,
    },
    [ImportAction.DraftUpdated]: {
      done: 'Draft updated',
      planned: 'Draft will be updated',
      tone: Tone.Info,
    },
    [ImportAction.SkippedExists]: {
      done: 'Skipped · name exists',
      planned: 'Skipped · name exists',
      tone: Tone.Off,
    },
    [ImportAction.Invalid]: {
      done: 'Not imported',
      planned: 'Cannot be imported',
      tone: Tone.Err,
    },
    [ImportAction.Failed]: {
      done: 'Failed',
      planned: 'Would fail',
      tone: Tone.Err,
    },
  };

  static label(item: ImportItem, dryRun: boolean): string {
    const meta = ImportReportModel.labels[item.action];
    return dryRun ? meta.planned : meta.done;
  }

  static tone(item: ImportItem): Tone {
    return ImportReportModel.labels[item.action].tone;
  }

  static issueTone(severity: IssueSeverity): Tone {
    return severity === IssueSeverity.Error ? Tone.Err : Tone.Warn;
  }

  /** "2 created · 1 skipped" style summary. */
  static summary(report: ImportReport): string {
    const counts = new Map<string, number>();
    report.items.forEach((item) => {
      const label = ImportReportModel.label(item, report.dryRun);
      counts.set(label, (counts.get(label) ?? 0) + 1);
    });
    return [...counts].map(([label, n]) => `${n} × ${label.toLowerCase()}`).join(' · ');
  }

  /** Whether a real import would write anything. */
  static writes(report: ImportReport | null): boolean {
    return !!report?.items.some((i) => i.action === ImportAction.Created || i.action === ImportAction.DraftUpdated);
  }
}
