import type { KeyboardEvent } from 'react';
import { Keys } from '../../../store';
import { BootstrapListParser } from '../model/BootstrapListParser';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import styles from './BrokersStep.module.css';

/** Keys handled by the broker chip input. */
const BrokerKeys = { Backspace: 'Backspace' } as const;

/** Step 2 — bootstrap servers as chips; a pasted list splits automatically; invalid entries turn red. */
export function BrokersStep({ form, set, errors }: ConnectionFormStepProps) {
  const add = (raw: string) => {
    set('servers', BootstrapListParser.add(form.servers, raw));
    set('brokerInput', '');
  };
  const onChange = (raw: string) => (BootstrapListParser.containsSeparator(raw) ? add(raw) : set('brokerInput', raw));
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === Keys.Enter && form.brokerInput) add(form.brokerInput);
    if (e.key === BrokerKeys.Backspace && !form.brokerInput) set('servers', form.servers.slice(0, -1));
  };
  const serverError = (index: number, server: string) =>
    errors[`${ConnectionField.Servers}.${index}`] ?? (BootstrapListParser.isValid(server) ? undefined : BootstrapListParser.invalidHint);
  const listError = errors[ConnectionField.Servers];
  return (
    <>
      <span className={styles.hint}>Bootstrap servers — paste a comma-separated list and it splits automatically.</span>
      <div className={styles.box}>
        {form.servers.map((server, i) => {
          const problem = serverError(i, server);
          return (
            <span key={server} className={[styles.chip, problem && styles.invalid].filter(Boolean).join(' ')} title={problem}>
              {server}
              <button type="button" className={styles.remove} aria-label={`Remove ${server}`} onClick={() => set('servers', form.servers.filter((s) => s !== server))}>
                ×
              </button>
            </span>
          );
        })}
        <input
          className={styles.input}
          value={form.brokerInput}
          placeholder="host:9092, host2:9092"
          aria-label="Bootstrap server"
          onChange={(e) => onChange(e.target.value)}
          onBlur={() => form.brokerInput && add(form.brokerInput)}
          onKeyDown={onKeyDown}
        />
      </div>
      {listError && <span className={styles.error}>{listError}</span>}
      {ConnectionFieldErrors.of(errors, ConnectionField.Servers) && !listError && (
        <span className={styles.error}>{ConnectionFieldErrors.of(errors, ConnectionField.Servers)}</span>
      )}
    </>
  );
}
