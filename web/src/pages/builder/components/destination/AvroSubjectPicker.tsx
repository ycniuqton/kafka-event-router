import { ControlSize, TextInput } from '../../../../components/ui';
import type { BuilderStepContext } from '../../model/BuilderStepContext';
import { SourceFormatOptions } from '../../model/SourceRules';
import styles from './DestinationOptionsList.module.css';

/** Schema subject of Avro output: suggests `<topic>-value` (the engine default) and warns without a registry. */
export function AvroSubjectPicker({ ctx }: { ctx: BuilderStepContext }) {
  const { draft, editor, connections } = ctx;
  const dest = draft.destination;
  const connection = connections.find((c) => c.id === dest.connectionId);
  const suggested = SourceFormatOptions.defaultSubject(dest.topic);
  return (
    <>
      <TextInput
        controlSize={ControlSize.Sm}
        mono
        className={styles.input}
        list="avro-subjects"
        aria-label="Schema subject"
        placeholder={suggested ? `Schema subject (default ${suggested})` : 'Schema subject, e.g. orders-value'}
        value={dest.schemaSubject ?? ''}
        onChange={(e) => editor.patchDestination({ schemaSubject: e.target.value }, 'Avro schema subject')}
      />
      <datalist id="avro-subjects">{suggested && <option value={suggested} />}</datalist>
      {connection && !connection.schemaRegistry && <div className={styles.dlq}>{SourceFormatOptions.registryWarning}</div>}
    </>
  );
}
