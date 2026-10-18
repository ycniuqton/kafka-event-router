import { useState } from 'react';
import { services } from '../../../../api';
import { useCan } from '../../../../access';
import { Button, EmptyState } from '../../../../components/ui';
import { Permission, type Route, type RouteSettings } from '../../../../domain';
import { actions, useServiceQuery } from '../../../../store';
import { SettingsField, SettingsForm, type SettingsFormValues } from '../../model/SettingsForm';
import { DeliverySection } from './DeliverySection';
import { ErrorPolicySection } from './ErrorPolicySection';
import { GeneralSection } from './GeneralSection';
import { LimitsSection } from './LimitsSection';
import { SettingsPanelRegistry } from './SettingsPanelRegistry';
import styles from './SettingsTab.module.css';

/** Route detail Settings tab (FR-44): loads the settings, then edits them in `SettingsEditor`. */
export function SettingsTab({ route }: { route: Route }) {
  const { data, error, reload } = useServiceQuery(() => services.routes.settings(route.id), [route.id]);
  if (error) {
    return <EmptyState compact title="Could not load settings" description={error.message} action={<Button size="sm" onClick={reload}>Retry</Button>} />;
  }
  if (!data) return <EmptyState compact title="Loading settings…" />;
  return <SettingsEditor key={JSON.stringify(data)} route={route} settings={data} onReload={reload} />;
}

type SettingsEditorProps = { route: Route; settings: RouteSettings; onReload: () => void };

/** The form: general metadata, error policy, delivery mode and limits, Save / Reset, plus panels registered by other features. */
function SettingsEditor({ route, settings, onReload }: SettingsEditorProps) {
  const canWrite = useCan(Permission.RouteWrite);
  const initial = SettingsForm.values(settings);
  const [values, setValues] = useState<SettingsFormValues>(initial);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const dirty = JSON.stringify(values) !== JSON.stringify(initial);
  const save = async () => {
    const invalid = SettingsForm.validate(values);
    setErrors(invalid);
    if (Object.keys(invalid).length) return;
    setSaving(true);
    try {
      await actions.routes.saveSettings(route.id, SettingsForm.patch(values));
      onReload();
    } catch (e) {
      setErrors(SettingsForm.failure(e));
    } finally {
      setSaving(false);
    }
  };
  const section = { values, errors, readOnly: !canWrite, onChange: (patch: Partial<SettingsFormValues>) => setValues((v) => ({ ...v, ...patch })) };
  return (
    <div className={styles.tab}>
      <GeneralSection {...section} />
      <ErrorPolicySection {...section} />
      <DeliverySection {...section} route={route} />
      <LimitsSection {...section} />
      {canWrite && (
        <div className={styles.footer}>
          {errors[SettingsField.Form] ? <span className={styles.formError}>{errors[SettingsField.Form]}</span> : <span className={styles.saved} />}
          {errors[SettingsField.Form] && <Button onClick={onReload}>Reload</Button>}
          <Button disabled={!dirty || saving} onClick={() => setValues(initial)}>Reset</Button>
          <Button variant="primary" disabled={!dirty || saving} onClick={() => void save()}>{saving ? 'Saving…' : 'Save settings'}</Button>
        </div>
      )}
      {SettingsPanelRegistry.all().map(({ id, Component }) => <Component key={id} route={route} />)}
    </div>
  );
}
