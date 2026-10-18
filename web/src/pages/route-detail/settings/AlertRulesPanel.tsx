import { useState } from 'react';
import { services } from '../../../api';
import { useCan } from '../../../access';
import { Button, Card, CardHeader, ToneBadge } from '../../../components/ui';
import { AlertRuleRules, ChannelTypeMeta, Permission, Tone, type AlertRule, type ChannelTestResult, type Route } from '../../../domain';
import { actions, useServiceQuery } from '../../../store';
import { AlertRuleForm } from './AlertRuleForm';
import { AlertRulesText } from './AlertRuleFormModel';
import styles from './AlertRules.module.css';

/** Editing state of the panel: nothing, a new rule, or one existing rule. */
const NewRule = 'new' as const;
type Editing = typeof NewRule | string | null;

/** Route Settings panel: the route's alert rules with add / edit / enable / test / delete (FR-16, FR-17). */
export function AlertRulesPanel({ route }: { route: Route }) {
  const canWrite = useCan(Permission.AlertWrite);
  const { data: rules, error, reload } = useServiceQuery(() => services.alerts.rules(route.id), [route.id]);
  const [editing, setEditing] = useState<Editing>(null);
  const [tests, setTests] = useState<Record<string, ChannelTestResult[]>>({});
  const saved = () => {
    setEditing(null);
    reload();
  };
  const toggle = async (rule: AlertRule) => {
    await services.alerts.saveRule({ draft: { ...AlertRuleRules.toDraft(route.id, AlertRuleRules.fromRule(rule)), enabled: !rule.enabled }, existing: rule });
    reload();
  };
  const test = async (rule: AlertRule) => {
    const results = await services.alerts.testRule(rule.id);
    setTests((t) => ({ ...t, [rule.id]: results }));
    actions.toast.show(AlertRulesText.testSent);
  };
  const remove = async (rule: AlertRule) => {
    if (!window.confirm(AlertRulesText.deleteConfirm)) return;
    await services.alerts.deleteRule(rule.id);
    reload();
  };
  return (
    <Card padding="md" className={styles.panel} aria-label={AlertRulesText.title}>
      <CardHeader title={AlertRulesText.title} meta={AlertRulesText.meta}
        actions={canWrite && editing === null ? <Button size="sm" onClick={() => setEditing(NewRule)}>{AlertRulesText.add}</Button> : undefined} />
      {error && <div className={styles.formError}>{AlertRulesText.loadFailed}: {error.message}</div>}
      {editing === NewRule && <AlertRuleForm routeId={route.id} onSaved={saved} onCancel={() => setEditing(null)} />}
      {rules?.length === 0 && editing !== NewRule && <div className={styles.empty}>{AlertRulesText.empty}</div>}
      {rules?.map((rule) =>
        editing === rule.id ? (
          <AlertRuleForm key={rule.id} routeId={route.id} rule={rule} onSaved={saved} onCancel={() => setEditing(null)} />
        ) : (
          <AlertRuleRow key={rule.id} rule={rule} canWrite={canWrite} results={tests[rule.id]} onEdit={() => setEditing(rule.id)}
            onToggle={() => void toggle(rule)} onTest={() => void test(rule)} onDelete={() => void remove(rule)} />
        ),
      )}
    </Card>
  );
}

type AlertRuleRowProps = {
  rule: AlertRule;
  canWrite: boolean;
  results?: ChannelTestResult[];
  onEdit: () => void;
  onToggle: () => void;
  onTest: () => void;
  onDelete: () => void;
};

/** One rule: name, condition, channels, last test results and its actions. */
function AlertRuleRow({ rule, canWrite, results, onEdit, onToggle, onTest, onDelete }: AlertRuleRowProps) {
  const channels = rule.channels.map((c) => ChannelTypeMeta[c.type].label).join(', ');
  return (
    <div className={styles.rule} data-disabled={!rule.enabled || undefined}>
      <div className={styles.ruleText}>
        <div className={styles.ruleName}>
          {rule.name} {!rule.enabled && <ToneBadge tone={Tone.Off}>{AlertRulesText.disabled}</ToneBadge>}
        </div>
        <div className={styles.ruleDetail}>{AlertRuleRules.summary(rule)} · {channels || AlertRulesText.noChannels}</div>
        {results?.map((r, i) => (
          <div key={i} className={styles.testResult} data-tone={r.ok ? Tone.Ok : Tone.Err}>
            {ChannelTypeMeta[r.channel].label}: {r.ok ? 'delivered' : r.error}
          </div>
        ))}
      </div>
      {canWrite && (
        <div className={styles.ruleActions}>
          <Button size="sm" variant="ghost" onClick={onToggle}>{rule.enabled ? 'Disable' : 'Enable'}</Button>
          <Button size="sm" variant="ghost" onClick={onTest} disabled={rule.channels.length === 0}>Send test</Button>
          <Button size="sm" variant="ghost" onClick={onEdit}>Edit</Button>
          <Button size="sm" variant="ghost" onClick={onDelete}>Delete</Button>
        </div>
      )}
    </div>
  );
}
