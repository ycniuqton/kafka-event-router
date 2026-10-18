import { useState } from 'react';
import { services } from '../../../api';
import { Button, Card, CardHeader, Field, Select, TextInput } from '../../../components/ui';
import {
  AlertRuleField,
  AlertRuleKind,
  AlertRuleKindMeta,
  AlertRuleRules,
  ChannelType,
  ChannelTypeMeta,
  type AlertRule,
  type AlertRuleFormValues,
  type ChannelFormValue,
} from '../../../domain';
import { AlertRuleFailure } from './AlertRuleFormModel';
import styles from './AlertRules.module.css';

type AlertRuleFormProps = { routeId: string; rule?: AlertRule; onSaved: (rule: AlertRule) => void; onCancel: () => void };

/** Create / edit one alert rule: kind, threshold, window, enabled and notification channels. */
export function AlertRuleForm({ routeId, rule, onSaved, onCancel }: AlertRuleFormProps) {
  const [values, setValues] = useState<AlertRuleFormValues>(() => (rule ? AlertRuleRules.fromRule(rule) : AlertRuleRules.empty()));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const patch = (p: Partial<AlertRuleFormValues>) => setValues((v) => ({ ...v, ...p }));
  const setKind = (kind: AlertRuleKind) => patch({ kind, threshold: String(AlertRuleKindMeta[kind].defaultThreshold) });
  const save = async () => {
    const invalid = AlertRuleRules.validate(values);
    setErrors(invalid);
    if (Object.keys(invalid).length) return;
    setSaving(true);
    try {
      const existing = rule ? { id: rule.id, version: rule.version } : undefined;
      onSaved(await services.alerts.saveRule({ draft: AlertRuleRules.toDraft(routeId, values), existing }));
    } catch (e) {
      setErrors(AlertRuleFailure.of(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Card padding="md" className={styles.form} aria-label="Alert rule">
      <CardHeader title={rule ? 'Edit alert rule' : 'New alert rule'} />
      <RuleFields values={values} errors={errors} onChange={patch} onKind={setKind} />
      <label className={styles.check}>
        <input type="checkbox" checked={values.enabled} onChange={(e) => patch({ enabled: e.target.checked })} /> Enabled
      </label>
      <ChannelsEditor channels={values.channels} errors={errors} onChange={(channels) => patch({ channels })} />
      <div className={styles.footer}>
        {errors[AlertRuleField.Form] ? <span className={styles.formError}>{errors[AlertRuleField.Form]}</span> : <span className={styles.spacer} />}
        <Button onClick={onCancel} disabled={saving}>Cancel</Button>
        <Button variant="primary" onClick={() => void save()} disabled={saving}>{saving ? 'Saving…' : 'Save rule'}</Button>
      </div>
    </Card>
  );
}

type RuleFieldsProps = {
  values: AlertRuleFormValues;
  errors: Record<string, string>;
  onChange: (p: Partial<AlertRuleFormValues>) => void;
  onKind: (kind: AlertRuleKind) => void;
};

/** Name, kind, threshold (hidden for "throughput is 0") and window. */
function RuleFields({ values, errors, onChange: patch, onKind: setKind }: RuleFieldsProps) {
  const meta = AlertRuleKindMeta[values.kind];
  return (
    <div className={styles.grid}>
      <Field label="Name" error={errors[AlertRuleField.Name]}>
        <TextInput block value={values.name} placeholder="Lag too high" onChange={(e) => patch({ name: e.target.value })} />
      </Field>
      <Field label="When" error={errors[AlertRuleField.Kind]}>
        <Select block value={values.kind} onChange={(e) => setKind(e.target.value as AlertRuleKind)}>
          {Object.values(AlertRuleKind).map((k) => (
            <option key={k} value={k}>{AlertRuleKindMeta[k].label}</option>
          ))}
        </Select>
      </Field>
      {meta.hasThreshold && (
        <Field label={meta.thresholdLabel} error={errors[AlertRuleField.Threshold]}>
          <TextInput block mono type="number" min={meta.min} max={meta.max} value={values.threshold} onChange={(e) => patch({ threshold: e.target.value })} />
        </Field>
      )}
      <Field label="For (minutes)" hint="Evaluated every 30 s over this window" error={errors[AlertRuleField.Window]}>
        <TextInput block mono type="number" min={AlertRuleRules.window.min} max={AlertRuleRules.window.max} value={values.windowMinutes} onChange={(e) => patch({ windowMinutes: e.target.value })} />
      </Field>
    </div>
  );
}

type ChannelsEditorProps = { channels: ChannelFormValue[]; errors: Record<string, string>; onChange: (channels: ChannelFormValue[]) => void };

/** Channel rows (type + recipients or write-only webhook URL) and the "Add channel" picker. */
function ChannelsEditor({ channels, errors, onChange }: ChannelsEditorProps) {
  const update = (i: number, p: Partial<ChannelFormValue>) => onChange(channels.map((c, j) => (j === i ? { ...c, ...p } : c)));
  return (
    <div className={styles.channels}>
      <div className={styles.channelsTitle}>Notify</div>
      {channels.map((c, i) => (
        <ChannelRow key={i} channel={c} error={errors[`${AlertRuleField.Channels}.${i}`]} onChange={(p) => update(i, p)} onRemove={() => onChange(channels.filter((_, j) => j !== i))} />
      ))}
      {errors[AlertRuleField.Channels] && <span className={styles.formError}>{errors[AlertRuleField.Channels]}</span>}
      {channels.length < AlertRuleRules.maxChannels && (
        <Select controlSize="sm" aria-label="Add channel" value="" onChange={(e) => onChange([...channels, AlertRuleRules.channel(e.target.value as ChannelType)])}>
          <option value="">+ Add channel</option>
          {Object.values(ChannelType).map((t) => (
            <option key={t} value={t}>{ChannelTypeMeta[t].label}</option>
          ))}
        </Select>
      )}
    </div>
  );
}

type ChannelRowProps = { channel: ChannelFormValue; error?: string; onChange: (p: Partial<ChannelFormValue>) => void; onRemove: () => void };

/** One channel; webhook URLs are write-only (a stored URL shows as "saved", typing replaces it). */
function ChannelRow({ channel, error, onChange, onRemove }: ChannelRowProps) {
  const meta = ChannelTypeMeta[channel.type];
  const label = meta.usesSecret ? `${meta.label} webhook URL` : `${meta.label} recipients`;
  return (
    <div className={styles.channel}>
      <span className={styles.channelType}>{meta.label}</span>
      <div className={styles.channelInput}>
        {meta.usesSecret ? (
          <TextInput block mono type="password" aria-label={label} autoComplete="off" value={channel.secret}
            placeholder={channel.secretSet ? '•••••• saved — type to replace' : meta.placeholder} onChange={(e) => onChange({ secret: e.target.value })} />
        ) : (
          <TextInput block mono aria-label={label} value={channel.target} placeholder={meta.placeholder} onChange={(e) => onChange({ target: e.target.value })} />
        )}
        {error && <span className={styles.formError}>{error}</span>}
      </div>
      <Button size="sm" variant="ghost" aria-label={`Remove ${meta.label} channel`} onClick={onRemove}>Remove</Button>
    </div>
  );
}
