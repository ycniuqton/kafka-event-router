import { useMemo, useState } from 'react';
import { useCan } from '../../../access';
import { services } from '../../../api';
import { Button, Drawer, EmptyState, TextInput } from '../../../components/ui';
import { ConnectionDisplay, Permission, type Connection } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { TopicRow } from '../components/TopicRow';
import { ConnectionDrawerTitle } from '../model/ConnectionDrawer';
import { DemoBadge } from '../components/DemoBadge';
import { CreateTopicDialog } from './CreateTopicDialog';
import { TopicBrowserModel, TopicBrowserText } from './TopicBrowserModel';
import styles from './TopicBrowser.module.css';

export type TopicBrowserDrawerProps = { connection: Connection; onClose: () => void };

/** Topics of a connection with search, internal toggle, rate, Peek and Create topic (spec §5.4, CN-50). */
export function TopicBrowserDrawer({ connection, onClose }: TopicBrowserDrawerProps) {
  const [peeking, setPeeking] = useState<string | null>(null);
  const [q, setQ] = useState('');
  const [includeInternal, setIncludeInternal] = useState(false);
  const [creating, setCreating] = useState(false);
  const query = useServiceQuery(() => services.connections.listTopics(connection.id, { includeInternal }), [connection.id, includeInternal]);
  const topics = useMemo(() => TopicBrowserModel.filter(query.data ?? [], q), [query.data, q]);
  const canCreate = TopicBrowserModel.canCreate(connection);
  const canReadRaw = useCan(Permission.SampleReadRaw);
  const onCreated = (name: string) => {
    setCreating(false);
    setQ(name);
    query.reload();
  };
  return (
    <Drawer open onClose={onClose} title={ConnectionDrawerTitle.topicsPrefix + ConnectionDisplay.name(connection)}>
      <div className={styles.toolbar}>
        <DemoBadge connection={connection} />
        <TextInput className={styles.search} controlSize="md" sunken value={q} placeholder={TopicBrowserText.search} aria-label={TopicBrowserText.search} onChange={(e) => setQ(e.target.value)} />
        <label className={styles.internalToggle}>
          <input type="checkbox" checked={includeInternal} onChange={(e) => setIncludeInternal(e.target.checked)} />
          {TopicBrowserText.showInternal}
        </label>
        <span title={canCreate ? undefined : TopicBrowserText.createDisabled}>
          <Button size="sm" disabled={!canCreate} onClick={() => setCreating(true)}>
            {TopicBrowserText.create}
          </Button>
        </span>
      </div>
      {query.loading && <EmptyState compact title={TopicBrowserText.loading} />}
      {query.error && (
        <div className={styles.problem} role="alert">
          {query.error.message}
        </div>
      )}
      {!query.loading && !query.error && !topics.length && <EmptyState compact title={q ? TopicBrowserText.noMatch : TopicBrowserText.empty} />}
      <div className={styles.list}>
        {topics.map((topic) => (
          <TopicRow
            key={topic.name}
            connectionId={connection.id}
            topic={topic}
            peeking={peeking === topic.name}
            sensitive={connection.sensitive ?? false}
            canReadRaw={canReadRaw}
            onPeek={() => setPeeking((p) => (p === topic.name ? null : topic.name))}
          />
        ))}
      </div>
      {creating && <CreateTopicDialog connectionId={connection.id} open onClose={() => setCreating(false)} onCreated={onCreated} />}
    </Drawer>
  );
}
