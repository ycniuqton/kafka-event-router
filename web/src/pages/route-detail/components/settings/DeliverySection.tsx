import { Card, CardHeader, SegmentedControl } from '../../../../components/ui';
import { DeliveryMode, Phase, RouteSettingsRules, Tone, type Route } from '../../../../domain';
import { PhaseTag } from '../PhaseTag';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

const deliveryOptions = Object.values(DeliveryMode).map((value) => ({ value, label: RouteSettingsRules.deliveryModeLabels[value] }));

/** Delivery guarantee (saved into the draft) with the read_committed explanation and the same-connection rule. */
export function DeliverySection({ route, values, onChange, readOnly }: SettingsSectionProps & { route: Route }) {
  const exactlyOnce = values.deliveryMode === DeliveryMode.ExactlyOnce;
  const foreign = RouteSettingsRules.foreignConnections(route.source.connectionId, [route.destination.connectionId], values.dlqConnectionId);
  return (
    <Card padding="md" className={styles.section}>
      <CardHeader title={<>Delivery <PhaseTag phase={Phase.P3} /></>} meta="Saved to the draft · applies on next deploy" />
      <SegmentedControl options={deliveryOptions} value={values.deliveryMode} onChange={(deliveryMode) => !readOnly && onChange({ deliveryMode })} ariaLabel="Delivery mode" />
      <p className={styles.hint}>{RouteSettingsRules.deliveryModeHints[values.deliveryMode]}</p>
      {exactlyOnce && foreign.length > 0 && (
        <div className={styles.notice} data-tone={Tone.Warn}>
          {RouteSettingsRules.exactlyOnceSameConnection}
        </div>
      )}
    </Card>
  );
}
