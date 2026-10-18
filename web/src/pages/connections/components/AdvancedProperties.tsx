import { Button, TextInput } from '../../../components/ui';
import { AllowedClientProperties, ClientProperties, type ClientProperty } from '../model/ConnectionForm';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import styles from './AdvancedProperties.module.css';

export type AdvancedPropertiesProps = { rows: ClientProperty[]; errors: Record<string, string>; onChange: (rows: ClientProperty[]) => void };

/** "Advanced" expander: raw Kafka client properties as key / value rows; rejected rows are highlighted. */
export function AdvancedProperties({ rows, errors, onChange }: AdvancedPropertiesProps) {
  const hasErrors = !!ConnectionFieldErrors.of(errors, ConnectionField.Properties);
  return (
    <details className={styles.details} open={hasErrors || undefined}>
      <summary className={styles.summary}>Advanced · raw client properties</summary>
      <div className={styles.rows}>
        <span className={styles.allowed}>Allowed: {AllowedClientProperties.join(', ')}</span>
        {rows.map((row) => {
          const error = ConnectionFieldErrors.ofProperty(errors, row.key.trim());
          return (
            <div key={row.id} className={[styles.row, error && styles.invalid].filter(Boolean).join(' ')} title={error}>
              <TextInput controlSize="sm" mono sunken value={row.key} placeholder="property" aria-label="Property name" list="connection-property-keys" onChange={(e) => onChange(ClientProperties.update(rows, row.id, { key: e.target.value }))} />
              <span className={styles.eq}>=</span>
              <TextInput controlSize="sm" mono sunken value={row.value} placeholder="value" aria-label="Property value" onChange={(e) => onChange(ClientProperties.update(rows, row.id, { value: e.target.value }))} />
              <Button size="sm" variant="ghost" square title="Remove property" onClick={() => onChange(ClientProperties.remove(rows, row.id))}>
                ×
              </Button>
              {error && <span className={styles.error}>{error}</span>}
            </div>
          );
        })}
        <datalist id="connection-property-keys">
          {AllowedClientProperties.map((k) => (
            <option key={k} value={k} />
          ))}
        </datalist>
        <Button size="sm" variant="ghost" className={styles.add} onClick={() => onChange(ClientProperties.add(rows))}>
          + Add property
        </Button>
      </div>
    </details>
  );
}
