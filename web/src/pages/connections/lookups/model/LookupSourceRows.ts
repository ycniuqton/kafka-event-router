import { Format } from '../../../../components/ui/Format';
import { LookupHealthMeta, LookupSourceKind, LookupSourceKindMeta, type LookupSource, type Tone } from '../../../../domain';

/** One row of the lookup sources table. */
export type LookupSourceRowView = {
  source: LookupSource;
  kindLabel: string;
  target: string;
  statusLabel: string;
  statusTone: Tone;
  statusTitle: string | undefined;
  checked: string;
  deleteTitle: string;
};

/** Kind / env / text filter of the table. */
export type LookupSourceFilterState = { kind: LookupSourceKind | null; text: string };

/** Builds row view-models and filters the list (LS-01). */
export class LookupSourceRows {
  static readonly initialFilter: LookupSourceFilterState = { kind: null, text: '' };
  static readonly deleteTitle = 'Delete lookup source';
  static readonly inUsePrefix = 'In use by: ';
  static readonly separator = ', ';
  static readonly lineBreak = '\n';

  static build(source: LookupSource, now: Date): LookupSourceRowView {
    const meta = LookupHealthMeta[source.health.status];
    const failure = source.health.lastFailure;
    return {
      source,
      kindLabel: LookupSourceKindMeta[source.kind].label,
      target: LookupSourceKindMeta[source.kind].target(source) || Format.placeholder,
      statusLabel: meta.label,
      statusTone: meta.tone,
      statusTitle: failure ? [failure.message, failure.hint].filter(Boolean).join(LookupSourceRows.lineBreak) : undefined,
      checked: Format.relativeTime(source.health.checkedAt, now),
      deleteTitle: source.usedBy.count ? LookupSourceRows.inUsePrefix + source.usedBy.routes.map((r) => r.name).join(LookupSourceRows.separator) : LookupSourceRows.deleteTitle,
    };
  }

  static filter(sources: LookupSource[], f: LookupSourceFilterState): LookupSource[] {
    const text = f.text.trim().toLowerCase();
    return sources.filter((s) => (!f.kind || s.kind === f.kind) && (!text || `${s.name} ${s.description}`.toLowerCase().includes(text)));
  }

  static kinds(): LookupSourceKind[] {
    return Object.values(LookupSourceKind);
  }
}
