import { Button, ButtonSize, ButtonVariant, Mono, TextSize } from '../../../../../components/ui';
import type { RuntimeInfo } from '../../../../../domain';
import styles from './SafetyRulesPanel.module.css';

/** Texts of the JS editor info panel that are not served by runtime-info. */
class SafetyPanelText {
  static readonly title = 'Safety rules';
  static readonly helpers = 'ctx helpers';
  static readonly loading = 'Loading sandbox rules…';
  static readonly failed = 'Could not load the sandbox rules.';
  static readonly retry = 'Retry';
  static readonly limitSeparator = ' · ';

  static limits(info: RuntimeInfo): string {
    const d = info.limits.defaults;
    return ['Limits: ' + d.jsTimeoutMs + ' ms per event', d.jsMemoryMb + ' MB memory', d.jsMaxOutputKb + ' KB output'].join(SafetyPanelText.limitSeparator);
  }
}

type SafetyRulesPanelProps = { info: RuntimeInfo | undefined; error: Error | undefined; onRetry(): void };

/** Info panel listing what custom code can and cannot do, the effective limits and the ctx helpers (spec §8.8). */
export function SafetyRulesPanel({ info, error, onRetry }: SafetyRulesPanelProps) {
  return (
    <div className={styles.panel} role="note">
      <div className={styles.title}>{SafetyPanelText.title}</div>
      {info ? <RuntimeInfoBody info={info} /> : <PanelStatus error={error} onRetry={onRetry} />}
    </div>
  );
}

function RuntimeInfoBody({ info }: { info: RuntimeInfo }) {
  return (
    <>
      <ul className={styles.list}>
        {info.safetyRules.map((rule) => (
          <li key={rule.id}>{rule.text}</li>
        ))}
      </ul>
      <div className={styles.limits}>{SafetyPanelText.limits(info)}</div>
      <div className={styles.title}>{SafetyPanelText.helpers}</div>
      <ul className={styles.helpers}>
        {info.helpers.map((helper) => (
          <li key={helper.name} title={helper.doc}>
            <Mono size={TextSize.Xs}>{`ctx.${helper.name}${helper.signature}`}</Mono>
            <span className={styles.doc}>{helper.doc}</span>
          </li>
        ))}
      </ul>
    </>
  );
}

function PanelStatus({ error, onRetry }: { error: Error | undefined; onRetry(): void }) {
  if (!error) return <div>{SafetyPanelText.loading}</div>;
  return (
    <div className={styles.status}>
      <span>{SafetyPanelText.failed}</span>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={onRetry}>
        {SafetyPanelText.retry}
      </Button>
    </div>
  );
}
