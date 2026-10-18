import { Card, CardHeader, EmptyState, Mono } from '../../../components/ui';
import { LibraryRules, Tone, type LibraryTransform } from '../../../domain';
import { LibraryLayout, LibraryText } from '../model/LibraryLayout';
import { LibraryRow } from './LibraryRow';
import styles from './LibraryList.module.css';

export type TransformsListProps = { transforms: LibraryTransform[]; loading: boolean; searching: boolean; onOpen(t: LibraryTransform): void };

/** Reusable transforms: name, version, description, used-by count and owner team. */
export function TransformsList({ transforms, loading, searching, onOpen }: TransformsListProps) {
  return (
    <Card>
      <CardHeader title="Transforms" meta={transforms.length} />
      {!loading && !transforms.length && (
        <div className={styles.empty}>
          <EmptyState compact title={searching ? LibraryText.noMatch : LibraryText.noTransforms} description={searching ? undefined : LibraryText.noTransformsHint} />
        </div>
      )}
      {transforms.map((t) => (
        <LibraryRow key={t.id} columns={LibraryLayout.columns} onOpen={() => onOpen(t)} label={t.name}>
          <Mono size="md" medium truncate>
            {t.name}
          </Mono>
          <Mono tone={Tone.Accent}>{t.version}</Mono>
          <span className={styles.secondary}>{t.description || LibraryRules.stepsLabel(t.size)}</span>
          <span className={styles.secondary}>{LibraryRules.usageLabel(t.usedBy)}</span>
          <span className={styles.muted}>{t.team}</span>
        </LibraryRow>
      ))}
    </Card>
  );
}
