import { StepKindLabel, ValueSourceKind, type TransformStepKind } from '../../../../domain';

/** Formatted copy of the Transform Studio, verbatim from the design. */
export class StudioText {
  static readonly missing = '?';
  static readonly noValue = '—';
  static readonly headerJoiner = '  ';
  static readonly lineBreak = '\n';
  static readonly readOnly = 'Read-only: your role can view these steps but not change them.';
  static readonly assistUnavailable = 'The assistant is unavailable right now. Your text is kept; try again in a moment.';

  static readonly emptyText = '""';
  static readonly choose = 'choose…';
  static lookupTable(name: string | undefined, version: number | undefined): string {
    return name ? `${name} v${version ?? '?'}` : StudioText.choose;
  }

  static lookupRows(count: number): string {
    return `${count} inline row${count === 1 ? '' : 's'}`;
  }

  static mapSummary(start: string, fields: number): string {
    return `${start} · ${fields} mapped field${fields === 1 ? '' : 's'}`;
  }
  static readonly constantPlaceholder = 'value, 42, true or "text"';
  static readonly formulaPlaceholder = 'concat(value.first, " ", value.last)';
  static readonly exampleMaxChars = 40;
  static readonly ellipsis = '…';
  static readonly noKey = '(no key)';
  static readonly sourcePrefix: Record<ValueSourceKind, string> = {
    [ValueSourceKind.Constant]: '',
    [ValueSourceKind.Field]: 'field ',
    [ValueSourceKind.Metadata]: 'meta ',
    [ValueSourceKind.Formula]: '= ',
  };
  static readonly previewPending = 'Running on samples…';
  static readonly previewFailed = 'Preview is unavailable';
  static readonly previewRetry = 'Retry';
  static readonly previewEngine = 'Output from the preview service (same engine as the workers).';

  /** Short label of the current event: `#n · <eventType|topic@offset>`. */
  static eventId(id: number, label: string): string {
    return label ? `#${StudioText.eventNumber(id)} · ${label}` : `#${StudioText.eventNumber(id)}`;
  }

  static exampleValue(value: unknown): string {
    const text = value === undefined ? StudioText.noValue : typeof value === 'string' ? value : JSON.stringify(value);
    return text.length > StudioText.exampleMaxChars ? text.slice(0, StudioText.exampleMaxChars) + StudioText.ellipsis : text;
  }

  static readonly noHeader = '(no header)';
  static readonly wholeValue = 'value';
  static readonly eachItem = 'one event per item';

  static headerExample(name: string, value: string | undefined): string {
    return value === undefined ? `${name || StudioText.missing} ${StudioText.noHeader}` : `${name}: ${StudioText.exampleValue(value)}`;
  }

  static itemCount(path: string, count: number): string {
    return `${path}: ${count} item${count === 1 ? '' : 's'}`;
  }

  static keyExample(key: string): string {
    return key ? `key ${StudioText.exampleValue(key)}` : StudioText.noKey;
  }

  static stepIssue(message: string): string {
    return `⚠ ${message} This step is skipped in the preview.`;
  }

  static eventNumber(id: number): number {
    return id + 1;
  }

  static position(index: number, total: number): string {
    return `Event ${index + 1} of ${total}`;
  }

  static runSummary(total: number, errors: number): string {
    return `${total} events · ${total - errors} ok · ${errors} error${errors === 1 ? '' : 's'}`;
  }

  static ranOnSamples(total: number, errors: number): string {
    return `Ran on ${total} samples · ${errors} error${errors === 1 ? '' : 's'}`;
  }

  static jumpTo(id: number): string {
    return `Jump to #${StudioText.eventNumber(id)}`;
  }

  static jumpToEvent(id: number): string {
    return `Jump to event ${StudioText.eventNumber(id)}`;
  }

  static eventError(id: number, message: string): string {
    return `Event #${StudioText.eventNumber(id)}: ${message}`;
  }

  static failedStep(index: number, kind: TransformStepKind): string {
    return `Step ${index + 1} · ${StepKindLabel[kind]}`;
  }

  static size(inputBytes: number, outputBytes: number): string {
    return `${inputBytes} B → ${outputBytes} B`;
  }

  static addedSteps(count: number): string {
    return `Added ${count} steps`;
  }

  static headers(headers: Record<string, string>): string {
    return Object.entries(headers)
      .map(([k, v]) => `${k}=${v}`)
      .join(StudioText.headerJoiner);
  }
}

/** Change labels reported to the builder with every step edit (design `setSteps` labels). */
export class StepChangeLabel {
  static added(kind: TransformStepKind): string {
    return `Added ${StepKindLabel[kind]}`;
  }

  static edited(kind: TransformStepKind): string {
    return `Edited ${StepKindLabel[kind]}`;
  }

  static removed(kind: TransformStepKind): string {
    return `Removed ${StepKindLabel[kind]}`;
  }

  static exported(count: number): string {
    return `Exported ${count} step${count === 1 ? '' : 's'} to JavaScript`;
  }

  static duplicated(kind: TransformStepKind): string {
    return `Duplicated ${StepKindLabel[kind]}`;
  }

  static toggled(kind: TransformStepKind, wasEnabled: boolean): string {
    return `${wasEnabled ? 'Disabled' : 'Enabled'} ${StepKindLabel[kind]}`;
  }

  static readonly reordered = 'Reordered steps';
  static readonly aiProposed = 'AI-proposed steps';
  static readonly editedJs = 'Edited JavaScript step';
}
