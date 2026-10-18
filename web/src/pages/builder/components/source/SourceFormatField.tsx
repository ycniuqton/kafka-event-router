import { ControlSize, Field, Select, TextInput } from '../../../../components/ui';
import { SourceFormatType, type Connection, type DraftSource } from '../../../../domain';
import type { DraftEditor } from '../../model/DraftSession';
import { SourceFormatOptions } from '../../model/SourceRules';
import styles from './SourceAdvanced.module.css';

type SourceFormatFieldProps = { source: DraftSource; connection: Connection | undefined; editor: DraftEditor };

/** Value format override (P2): JSON by default; Avro / Protobuf with schema subject and message type. */
export function SourceFormatField({ source, connection, editor }: SourceFormatFieldProps) {
  const format = source.valueFormat ?? { type: SourceFormatType.Json };
  const patch = (next: Partial<typeof format>, change: string) => editor.patchSource({ valueFormat: { ...format, ...next } }, change);
  const registry = SourceFormatOptions.usesRegistry(format.type);
  return (
    <Field label="Format">
      <Select controlSize={ControlSize.Lg} value={format.type} aria-label="Source format" onChange={(e) => patch({ type: e.target.value as SourceFormatType }, `Source format: ${e.target.value}`)}>
        {SourceFormatOptions.options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </Select>
      {registry && (
        <div className={styles.subject}>
          <TextInput
            controlSize={ControlSize.Sm}
            mono
            aria-label="Schema subject"
            placeholder={SourceFormatOptions.defaultSubject(source.topics[0]) || 'subject (default <topic>-value)'}
            value={format.subject ?? ''}
            onChange={(e) => patch({ subject: e.target.value }, 'Source schema subject')}
          />
          {format.type === SourceFormatType.Protobuf && (
            <TextInput controlSize={ControlSize.Sm} mono aria-label="Message type" placeholder="message type, e.g. acme.Order" value={format.messageType ?? ''} onChange={(e) => patch({ messageType: e.target.value }, 'Protobuf message type')} />
          )}
        </div>
      )}
      {registry && connection && !connection.schemaRegistry && <div className={styles.warn}>{SourceFormatOptions.registryWarning}</div>}
    </Field>
  );
}
