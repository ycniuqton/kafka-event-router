import { LookupSourceKindMeta, type LookupSource } from '../../../../domain';

/**
 * Lookup sources known to the builder (loaded once per page by `useLookupSourceCatalog`), so step summaries and
 * the pipeline rail can show a source's name and kind instead of its id.
 */
export class LookupSourceCatalog {
  private static sources = new Map<string, LookupSource>();
  static readonly unknownLabel = 'lookup source';

  static load(list: LookupSource[]): void {
    LookupSourceCatalog.sources = new Map(list.map((s) => [s.id, s]));
  }

  static get(id: string): LookupSource | undefined {
    return LookupSourceCatalog.sources.get(id);
  }

  static all(): LookupSource[] {
    return [...LookupSourceCatalog.sources.values()];
  }

  /** `crm-api (HTTP)`, or the raw id while the list is loading / for an unknown source. */
  static label(id: string): string {
    const s = LookupSourceCatalog.sources.get(id);
    if (!s) return id || LookupSourceCatalog.unknownLabel;
    return `${s.name} (${LookupSourceKindMeta[s.kind].label})`;
  }
}
