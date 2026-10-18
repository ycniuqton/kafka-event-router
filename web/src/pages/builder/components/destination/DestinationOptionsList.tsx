import { ControlSize, Select, TextInput } from '../../../../components/ui';
import { DeadLetterPolicy, DestinationOptions, RouteSettingsRules, type DraftDestination } from '../../../../domain';
import type { BuilderStepContext } from '../../model/BuilderStepContext';
import { DestinationExtrasRules } from '../../model/DestinationExtras';
import { DestinationRules } from '../../model/ReviewModel';
import { DestinationOptionRow } from './DestinationOptionRow';
import { AvroSubjectPicker } from './AvroSubjectPicker';
import { HeaderRowsEditor } from './HeaderRowsEditor';
import styles from './DestinationOptionsList.module.css';

type OptionKey = keyof typeof DestinationOptions & keyof DraftDestination;

/** Key, partitioning, headers, output format and on-error choices with their dependent inputs (all saved in the draft). */
export function DestinationOptionsList({ ctx }: { ctx: BuilderStepContext }) {
  const { draft, editor, catalog } = ctx;
  const dest = draft.destination;
  const choose = (key: OptionKey, label: string) => (value: string) => editor.patchDestination({ [key]: value }, `${label}: ${value}`);
  const keyField = dest.keyField || DestinationExtrasRules.defaultKeyField;
  return (
    <div className={styles.list}>
      <DestinationOptionRow label="Key" options={DestinationOptions.key} value={dest.key} onChange={choose('key', 'Key')}>
        {dest.key === DestinationExtrasRules.keyFromField && (
          <Select controlSize={ControlSize.Sm} mono className={styles.input} value={keyField} onChange={(e) => editor.patchDestination({ keyField: e.target.value }, `Key from ${e.target.value}`)}>
            {catalog.pathsFor(keyField).map((p) => (
              <option key={p}>{p}</option>
            ))}
          </Select>
        )}
      </DestinationOptionRow>
      <DestinationOptionRow label="Partitioning" options={DestinationOptions.partitioning} value={dest.partitioning} onChange={choose('partitioning', 'Partitioning')} />
      <DestinationOptionRow label="Headers" options={DestinationOptions.headers} value={dest.headers} onChange={choose('headers', 'Headers')}>
        {dest.headers === DestinationExtrasRules.keepSelected && (
          <TextInput
            controlSize={ControlSize.Sm}
            mono
            className={styles.input}
            placeholder="Header names to keep, e.g. traceId, tenant"
            value={(dest.keepHeaders ?? []).join(`${RouteSettingsRules.tagSeparator} `)}
            onChange={(e) => editor.patchDestination({ keepHeaders: RouteSettingsRules.parseTags(e.target.value) }, 'Kept headers changed')}
          />
        )}
        <HeaderRowsEditor rows={dest.addHeaders ?? []} onChange={(addHeaders, change) => editor.patchDestination({ addHeaders }, change)} />
      </DestinationOptionRow>
      <DestinationOptionRow label="Output format" options={DestinationRules.formats()} value={dest.format} onChange={choose('format', 'Output format')}>
        {dest.format === DestinationRules.avroLabel && <AvroSubjectPicker ctx={ctx} />}
      </DestinationOptionRow>
      <DestinationOptionRow label="On error" options={DestinationOptions.onError} value={dest.onError} onChange={choose('onError', 'On error')}>
        {dest.onError === DeadLetterPolicy && <div className={styles.dlq}>→ {DestinationRules.dlqName(draft.name)} · change the dead-letter topic in route Settings</div>}
      </DestinationOptionRow>
    </div>
  );
}
