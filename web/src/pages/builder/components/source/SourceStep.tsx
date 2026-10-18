import { useState } from 'react';
import { ConnectionStatusMeta, ConnectionStatus, StartPosition, type Connection, type RouteDraft } from '../../../../domain';
import { Field, HealthDot, Select, TextInput, ControlSize } from '../../../../components/ui';
import type { DraftEditor } from '../../model/DraftSession';
import type { StepEditorProps } from '../StepEditorProps';
import { ConnectionOption, ConsumerGroupName } from '../../model/SourceRules';
import { StepHeading } from '../StepHeading';
import { OffsetsEditor } from './OffsetsEditor';
import { SourceFormatField } from './SourceFormatField';
import { StartFromField } from './StartFromField';
import { TopicPicker } from './TopicPicker';
import styles from './SourceStep.module.css';

/** Source step (spec §7.1): connection with health, topics or a pattern, start position (incl. offsets), format, consumer group. */
export function SourceStep({ ctx }: StepEditorProps) {
  const { draft, editor, connections } = ctx;
  const source = draft.source;
  const connection = connections.find((c) => c.id === source.connectionId);
  return (
    <div className={styles.step}>
      <StepHeading title="Source" subtitle="Where the route reads events from." />
      <SourceConnectionField draft={draft} connection={connection} connections={connections} editor={editor} />
      <TopicPicker source={source} topics={connection?.topics ?? []} editor={editor} />
      <StartFromField
        value={source.start}
        at={source.startAt}
        onChange={(start) => editor.patchSource({ start }, `Start position: ${start}`)}
        onAt={(startAt) => editor.patchSource({ startAt }, 'Start time changed')}
      />
      {source.start === StartPosition.Offsets && <OffsetsEditor source={source} topics={connection?.topics ?? []} editor={editor} />}
      <div className={styles.grid}>
        <SourceFormatField source={source} connection={connection} editor={editor} />
        <ConsumerGroupField draft={draft} editor={editor} />
      </div>
    </div>
  );
}

type ConnectionFieldProps = { draft: RouteDraft; connection: Connection | undefined; connections: Connection[]; editor: DraftEditor };

/** Connection select with its health dot; a new draft's destination follows the source until changed. */
function SourceConnectionField({ draft, connection, connections, editor }: ConnectionFieldProps) {
  const source = draft.source;
  const health = ConnectionStatusMeta[connection?.status ?? ConnectionStatus.Untested];
  const changeConnection = (id: string) => {
    editor.patchSource({ connectionId: id, topics: [] }, 'Source connection changed');
    if (!draft.routeId && draft.destination.connectionId === source.connectionId) editor.patchDestination({ connectionId: id }, 'Destination connection follows source');
  };
  return (
    <Field label="Connection">
      <div className={styles.connRow}>
        <Select controlSize={ControlSize.Lg} mono className={styles.connSelect} value={source.connectionId} onChange={(e) => changeConnection(e.target.value)}>
          {!connection && <option value={source.connectionId}>{source.connectionId || 'Choose a connection'}</option>}
          {connections.map((c) => (
            <option key={c.id} value={c.id}>
              {ConnectionOption.label(c)}
            </option>
          ))}
        </Select>
        <HealthDot tone={health.tone} label={health.label} large />
      </div>
    </Field>
  );
}

/** Consumer group: read-only default, editable under "Advanced". */
function ConsumerGroupField({ draft, editor }: { draft: RouteDraft; editor: DraftEditor }) {
  const [advanced, setAdvanced] = useState(false);
  const source = draft.source;
  return (
    <Field label={<ConsumerGroupLabel advanced={advanced} onToggle={() => setAdvanced((a) => !a)} />} hint={advanced ? 'Changing it restarts reading from the start position.' : undefined}>
      <TextInput
        controlSize={ControlSize.Lg}
        mono
        sunken={!advanced}
        readOnly={!advanced}
        className={advanced ? undefined : styles.group}
        value={advanced ? (source.consumerGroup ?? '') : ConsumerGroupName.effective(source, draft.name)}
        placeholder={ConsumerGroupName.of(draft.name)}
        onChange={(e) => editor.patchSource({ consumerGroup: e.target.value }, 'Consumer group changed')}
      />
    </Field>
  );
}

/** "Consumer group · Advanced" label; the toggle makes the group editable. */
function ConsumerGroupLabel({ advanced, onToggle }: { advanced: boolean; onToggle: () => void }) {
  return (
    <span className={styles.groupLabel}>
      Consumer group
      <button
        type="button"
        className={styles.advanced}
        onClick={(e) => {
          e.preventDefault();
          onToggle();
        }}
      >
        {advanced ? 'Done' : 'Advanced · edit'}
      </button>
    </span>
  );
}
