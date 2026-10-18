import { useState } from 'react';
import { services } from '../../../api';
import { Button, Drawer } from '../../../components/ui';
import { ConnectionDisplay, type Connection, type ConnectionTestOptions } from '../../../domain';
import { useConnectionTest, useServiceQuery } from '../../../store';
import { ConnectionDrawerTitle } from '../model/ConnectionDrawer';
import { TestChecklist } from '../test/TestChecklist';
import { TestTopicPickers } from '../test/TestTopicPickers';
import styles from './ConnectionTestDrawer.module.css';

export type ConnectionTestDrawerProps = {
  connection: Connection;
  onClose: () => void;
  onRetest: (id: string, options: ConnectionTestOptions) => void;
};

/** Live checklist of the connection test with plain-language failure hints and read/write topic pickers (spec §5.3). */
export function ConnectionTestDrawer({ connection, onClose, onRetest }: ConnectionTestDrawerProps) {
  const run = useConnectionTest(connection.id);
  const [options, setOptions] = useState<ConnectionTestOptions>({});
  const { data: topics = [] } = useServiceQuery(() => services.connections.listTopics(connection.id), [connection.id]);
  const running = !!run && !run.finished;
  return (
    <Drawer open onClose={onClose} title={ConnectionDrawerTitle.testPrefix + ConnectionDisplay.name(connection)}>
      <TestChecklist run={run} />
      {topics.length > 0 && <TestTopicPickers topics={topics.map((t) => t.name)} options={options} disabled={running} onChange={setOptions} />}
      {(!run || run.finished) && (
        <Button className={styles.again} onClick={() => onRetest(connection.id, options)}>
          {run ? 'Test again' : 'Test'}
        </Button>
      )}
    </Drawer>
  );
}
