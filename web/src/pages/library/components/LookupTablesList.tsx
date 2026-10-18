import { Card, CardHeader, EmptyState, Format, Mono } from '../../../components/ui';
import { LibraryRules, Tone, type LibraryLookupTable } from '../../../domain';
import { LibraryLayout, LibraryText } from '../model/LibraryLayout';
import { LibraryRow } from './LibraryRow';
import styles from './LibraryList.module.css';

export type LookupTablesListProps = { tables: LibraryLookupTable[]; loading: boolean; searching: boolean; onOpen(t: LibraryLookupTable): void };

/** Versioned lookup tables, stored the same way as transforms. */
export function LookupTablesList({ tables, loading, searching, onOpen }: LookupTablesListProps) {
  return (
    <Card>
      <CardHeader title="Lookup tables" meta={tables.length} />
      {!loading && !tables.length && (
        <div className={styles.empty}>
          <EmptyState compact title={searching ? LibraryText.noMatch : LibraryText.noTables} description={searching ? undefined : LibraryText.noTablesHint} />
        </div>
      )}
      {tables.map((t) => (
        <LibraryRow key={t.id} columns={LibraryLayout.lookupColumns} onOpen={() => onOpen(t)} label={t.name}>
          <Mono size="md" medium truncate>
            {t.name}
          </Mono>
          <Mono tone={Tone.Accent}>{t.version}</Mono>
          <span className={styles.secondary}>{t.description}</span>
          <Mono tone={Tone.Muted}>{Format.number(t.rows)} rows</Mono>
          <span className={styles.secondary}>{LibraryRules.usageLabel(t.usedBy)}</span>
          <span className={styles.muted}>{t.team}</span>
        </LibraryRow>
      ))}
    </Card>
  );
}
