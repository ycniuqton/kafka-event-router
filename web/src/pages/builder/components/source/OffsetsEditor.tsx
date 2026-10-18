import { TextInput, ControlSize } from '../../../../components/ui';
import type { DraftSource, TopicInfo } from '../../../../domain';
import type { DraftEditor } from '../../model/DraftSession';
import { OffsetRows } from '../../model/SourceRules';
import styles from './SourceAdvanced.module.css';

type OffsetsEditorProps = { source: DraftSource; topics: TopicInfo[]; editor: DraftEditor };

/** Start offset per partition (P2 "specific offsets"); empty partitions start from the committed / latest offset. */
export function OffsetsEditor({ source, topics, editor }: OffsetsEditorProps) {
  const partitions = OffsetRows.partitions(source, topics);
  const set = (partition: string, raw: string) => editor.patchSource({ offsets: OffsetRows.set(source.offsets, partition, raw) }, `Start offset of partition ${partition}`);
  if (!partitions.length) return <div className={styles.hint}>Pick one topic to set an offset per partition.</div>;
  return (
    <div className={styles.offsets}>
      {partitions.map((p) => (
        <label key={p} className={styles.offset}>
          p{p}
          <TextInput
            controlSize={ControlSize.Sm}
            mono
            className={styles.offsetInput}
            inputMode="numeric"
            placeholder="offset"
            aria-label={`Offset of partition ${p}`}
            value={source.offsets?.[p] ?? ''}
            onChange={(e) => set(p, e.target.value)}
          />
        </label>
      ))}
    </div>
  );
}
