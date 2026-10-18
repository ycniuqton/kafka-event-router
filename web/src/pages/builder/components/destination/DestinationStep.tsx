import { ControlSize, Field, Select, TextInput } from '../../../../components/ui';
import { EnvTag } from '../../../../domain';
import { actions } from '../../../../store';
import type { StepEditorProps } from '../StepEditorProps';
import { DestinationRules } from '../../model/ReviewModel';
import { ConnectionOption } from '../../model/SourceRules';
import { StepHeading } from '../StepHeading';
import { CreateTopicNotice } from './CreateTopicNotice';
import { DestinationOptionsList } from './DestinationOptionsList';
import styles from './DestinationStep.module.css';

/** Destination step (spec §7.3): connection, topic (create if missing), key, partitioning, headers, format, errors. */
export function DestinationStep({ ctx }: StepEditorProps) {
  const { draft, editor, connections } = ctx;
  const dest = draft.destination;
  const connection = connections.find((c) => c.id === dest.connectionId);
  const setTopic = (topic: string) => editor.patchDestination({ topic }, 'Destination topic changed');
  const missing = !!dest.topic && !!connection && !DestinationRules.topicExists(connection, dest.topic);
  const createOnDeploy = (on: boolean) =>
    editor.patchDestination({ createTopic: on ? { partitions: DestinationRules.newTopicPartitions } : null }, on ? 'Create topic on deploy' : 'Do not create topic');
  return (
    <div className={styles.step}>
      <StepHeading title="Destination" subtitle="Where the result is written." />
      <div className={styles.grid}>
        <Field label="Connection">
          <Select controlSize={ControlSize.Lg} mono className={styles.mono13} value={dest.connectionId} onChange={(e) => editor.patchDestination({ connectionId: e.target.value }, 'Destination connection changed')}>
            {!connection && <option value={dest.connectionId}>{dest.connectionId || 'Choose a connection'}</option>}
            {connections.map((c) => (
              <option key={c.id} value={c.id}>
                {ConnectionOption.label(c)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Topic">
          <TextInput controlSize={ControlSize.Lg} mono className={styles.mono13} value={dest.topic} placeholder="topic name" list="destination-topics" onChange={(e) => setTopic(e.target.value)} />
        </Field>
      </div>
      <datalist id="destination-topics">
        {connection?.topics.filter((t) => !t.internal).map((t) => <option key={t.name} value={t.name} />)}
      </datalist>
      {missing && (
        <CreateTopicNotice
          topic={dest.topic}
          allowed={connection?.allowTopicCreate !== false}
          onDeploy={!!dest.createTopic}
          onToggleDeploy={createOnDeploy}
          onCreate={() => actions.connections.createTopic(dest.connectionId, dest.topic, { partitions: DestinationRules.newTopicPartitions })}
        />
      )}
      {connection?.env === EnvTag.Prod && <div className={styles.prod}>● Production connection</div>}
      <DestinationOptionsList ctx={ctx} />
    </div>
  );
}
