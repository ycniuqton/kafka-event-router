import { DestinationOptions, type HeaderRow } from '../../../domain';

/** Legacy session-only destination extras; the values now live in `RouteDraft.destination` (kept for the context shape). */
export type DestinationExtras = { keyField: string; headers: HeaderRow[]; schemaSubject: string };

/** Defaults and option values that unlock each dependent destination input. */
export class DestinationExtrasRules {
  static readonly initial: DestinationExtras = { keyField: '', headers: [], schemaSubject: '' };
  static readonly defaultKeyField = 'value.id';
  static readonly keyFromField = DestinationOptions.key[1];
  static readonly keepSelected = DestinationOptions.headers[1];
  static readonly avro = DestinationOptions.format[2];
  static readonly blankHeader: HeaderRow = { key: '', value: '' };
}
