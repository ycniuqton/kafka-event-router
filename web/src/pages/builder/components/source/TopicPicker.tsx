import { useState } from 'react';
import type { DraftSource, TopicInfo } from '../../../../domain';
import type { DraftEditor } from '../../model/DraftSession';
import { TopicPattern } from '../../model/SourceRules';
import { TopicPatternField } from './TopicPatternField';
import { TopicRow } from './TopicRow';
import styles from './TopicPicker.module.css';

export type TopicPickerProps = { source: DraftSource; topics: TopicInfo[]; editor: DraftEditor };

/** Searchable topic list with partitions and rates, single or multi-select (merge), or a regex pattern (P2); internal topics are hidden. */
export function TopicPicker({ source, topics, editor }: TopicPickerProps) {
  const [query, setQuery] = useState('');
  const [patternMode, setPatternMode] = useState(false);
  const visible = topics.filter((t) => !t.internal);
  const toggle = (name: string) => {
    const on = source.topics.includes(name);
    const multiTopics = on ? source.topics.filter((t) => t !== name) : [...source.topics, name];
    editor.patchSource({ topics: source.multi ? multiTopics : [name] }, 'Source topic changed');
  };
  const toggleMulti = () => editor.adjustSource({ multi: !source.multi, topics: source.multi ? source.topics.slice(0, 1) : source.topics });
  const byPattern = source.topicPattern !== undefined && source.topicPattern !== '' ? true : patternMode;
  const togglePattern = () => {
    setPatternMode(!byPattern);
    if (byPattern) editor.patchSource({ topicPattern: '' }, 'Topic pattern removed');
  };
  return (
    <div className={styles.picker}>
      <div className={styles.header}>
        <span className={styles.label}>{byPattern ? 'Topic pattern' : `Topic${source.multi ? 's' : ''}`}</span>
        {!byPattern && (
          <button type="button" className={styles.link} onClick={toggleMulti}>
            Multiple topics (merge) · {source.multi ? 'on' : 'off'}
          </button>
        )}
        <button type="button" className={styles.link} onClick={togglePattern}>
          Pattern (regex) · {byPattern ? 'on' : 'off'}
        </button>
      </div>
      {byPattern ? <TopicPatternField source={source} topics={visible} editor={editor} /> : <TopicList source={source} visible={visible} query={query} setQuery={setQuery} toggle={toggle} />}
    </div>
  );
}

type TopicListProps = { source: DraftSource; visible: TopicInfo[]; query: string; setQuery: (q: string) => void; toggle: (name: string) => void };

/** Searchable list of topics with the selected-but-missing ones first. */
function TopicList({ source, visible, query, setQuery, toggle }: TopicListProps) {
  const shown = TopicPattern.filter(visible, query);
  const missing = source.topics.filter((name) => !visible.some((t) => t.name === name));
  return (
      <div className={styles.box}>
        <input className={styles.search} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search topics" aria-label="Search topics" />
        {missing.map((name) => (
          <TopicRow key={name} topic={{ name, partitions: 0, rate: 'not found' }} selected multi={source.multi} onClick={() => toggle(name)} />
        ))}
        {shown.map((t) => (
          <TopicRow key={t.name} topic={t} selected={source.topics.includes(t.name)} multi={source.multi} onClick={() => toggle(t.name)} />
        ))}
        {!shown.length && <div className={styles.empty}>{visible.length ? 'No topic matches this search.' : 'No topics on this connection yet.'}</div>}
      </div>
  );
}
