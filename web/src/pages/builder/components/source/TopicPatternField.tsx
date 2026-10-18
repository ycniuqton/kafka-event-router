import type { DraftSource, TopicInfo } from '../../../../domain';
import type { DraftEditor } from '../../model/DraftSession';
import { TopicPattern } from '../../model/SourceRules';
import styles from './SourceAdvanced.module.css';
import pickerStyles from './TopicPicker.module.css';

type TopicPatternFieldProps = { source: DraftSource; topics: TopicInfo[]; editor: DraftEditor };

/** Regex subscription (P2): the route reads every topic matching the pattern, including topics created later. */
export function TopicPatternField({ source, topics, editor }: TopicPatternFieldProps) {
  const pattern = source.topicPattern ?? '';
  const matches = TopicPattern.matches(topics, pattern);
  return (
    <div className={pickerStyles.box}>
      <input
        className={pickerStyles.search}
        value={pattern}
        onChange={(e) => editor.patchSource({ topicPattern: e.target.value }, 'Topic pattern changed')}
        placeholder="Regular expression, e.g. ^orders\..*$"
        aria-label="Topic pattern"
      />
      {matches === null && <div className={styles.error}>Not a valid regular expression.</div>}
      {matches && (
        <div className={styles.matches}>
          {matches.length ? matches.map((t) => <span key={t.name}>{t.name}</span>) : <span>No topic matches yet; new matching topics are picked up while running.</span>}
        </div>
      )}
    </div>
  );
}
