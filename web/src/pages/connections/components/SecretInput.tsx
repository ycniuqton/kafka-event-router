import { ConnectionFormModel, type SecretValue } from '../model/ConnectionForm';
import { TextInput } from '../../../components/ui';
import styles from './SecretInput.module.css';

export type SecretInputProps = { value: SecretValue; placeholder: string; error?: string; onChange: (value: SecretValue) => void };

/** Write-only secret: once saved it shows as •••••• with a Replace link and is never revealed. */
export function SecretInput({ value, placeholder, error, onChange }: SecretInputProps) {
  if (value.stored) {
    return (
      <div className={styles.stored}>
        <span className={styles.dots}>••••••</span>
        <button type="button" className={styles.link} onClick={() => onChange(ConnectionFormModel.emptySecret)}>
          Replace
        </button>
        {error && <span className={styles.error}>{error}</span>}
      </div>
    );
  }
  return (
    <>
      <TextInput
        type="password"
        controlSize="lg"
        sunken
        autoComplete="new-password"
        value={value.value}
        placeholder={placeholder}
        aria-label={placeholder}
        onChange={(e) => onChange({ value: e.target.value, stored: false })}
      />
      {error && <span className={styles.error}>{error}</span>}
    </>
  );
}
