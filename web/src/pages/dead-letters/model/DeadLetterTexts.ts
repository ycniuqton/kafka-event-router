import { DeadLetterJobAction, type DeadLetterOriginal } from '../../../domain';

/** How the original event is shown: JSON tree, or raw JSON text (users who may read raw values only). */
export const OriginalView = { Tree: 'tree', Raw: 'raw' } as const;
export type OriginalView = (typeof OriginalView)[keyof typeof OriginalView];

/** Labels and layout of the original-event panel. */
export class OriginalText {
  static readonly options = [
    { value: OriginalView.Tree, label: 'Tree' },
    { value: OriginalView.Raw, label: 'Raw' },
  ];
  static readonly indent = 2;

  static section(original: DeadLetterOriginal | undefined): string {
    const schema = original?.schema ? ` · ${original.schema.format}${original.schema.schemaId ? ` schema #${original.schema.schemaId}` : ''}` : '';
    return `Original event${original?.rawBytes ? ' · raw bytes (base64)' : ''}${schema}`;
  }
}

/** Texts of the bulk-job prompt. */
export class BulkJobText {
  static title(action: DeadLetterJobAction): string {
    return action === DeadLetterJobAction.Retry ? 'Retry all in the background?' : 'Discard all in the background?';
  }

  static body(action: DeadLetterJobAction, routeName: string, count: string): string {
    const verb = action === DeadLetterJobAction.Retry ? 'retried through the active version' : 'discarded';
    return `${count} matching dead letters of ${routeName} will be ${verb} by a background job. You can leave this page; the alerts menu tells you when it finishes.`;
  }
}
