import { useState } from 'react';
import { services } from '../../../api';
import { Can, useCan } from '../../../access';
import { Mono, SegmentedControl, ToneBadge } from '../../../components/ui';
import { Permission, Theme, type WorkspaceSettings } from '../../../domain';
import { actions, settingsStore, useCurrentUser, useServiceQuery, useSettings } from '../../../store';
import { PhaseTagChoice } from '../model/SettingsCatalog';
import { AssistChoice, AuditExportView, WorkspaceText } from '../model/WorkspaceSettingsModel';
import { SettingRow } from './SettingRow';
import styles from './SettingsSection.module.css';

const ThemeOptions = [
  { value: Theme.Dark, label: 'Dark' },
  { value: Theme.Light, label: 'Light' },
];

const PhaseOptions = [
  { value: PhaseTagChoice.Show, label: 'Show' },
  { value: PhaseTagChoice.Hide, label: 'Hide' },
];

const AssistOptions = [
  { value: AssistChoice.On, label: WorkspaceText.assistOn },
  { value: AssistChoice.Off, label: WorkspaceText.assistOff },
];

/** Workspace name, the persisted theme and phase-tag preferences, AI assist and the audit export status. */
export function WorkspaceSection() {
  const settings = useSettings();
  const user = useCurrentUser();
  return (
    <>
      <SettingRow label="Workspace">
        <Mono size="md">{user?.workspace}</Mono>
      </SettingRow>
      <SettingRow label="Theme">
        <SegmentedControl size="md" options={ThemeOptions} value={settings.theme} onChange={(t) => settingsStore.setTheme(t)} ariaLabel="Theme" />
      </SettingRow>
      <SettingRow label="Phase tags">
        <SegmentedControl
          size="md"
          options={PhaseOptions}
          value={settings.showPhases ? PhaseTagChoice.Show : PhaseTagChoice.Hide}
          onChange={(v) => settingsStore.setShowPhases(v === PhaseTagChoice.Show)}
          ariaLabel="Phase tags"
        />
        <span className={styles.hint}>MVP / P2 / P3 markers in the navigation</span>
      </SettingRow>
      <AssistRow />
      <Can permission={Permission.AuditRead}>
        <AuditExportRow />
      </Can>
    </>
  );
}

/** Workspace-wide AI assist switch: admins change it (If-Match), everyone else sees its state. */
function AssistRow() {
  const canManage = useCan(Permission.SettingsManage);
  const { data } = useServiceQuery(() => services.access.settings(), []);
  const [saved, setSaved] = useState<WorkspaceSettings | null>(null);
  const current = saved ?? data;
  if (!current) return null;
  const change = async (choice: AssistChoice) => {
    try {
      setSaved(await services.access.updateSettings(current, { aiAssistEnabled: choice === AssistChoice.On }));
      actions.toast.show(WorkspaceText.saved);
    } catch (e) {
      actions.toast.show((e as Error).message);
    }
  };
  const value = current.aiAssistEnabled ? AssistChoice.On : AssistChoice.Off;
  return (
    <SettingRow label="AI assist">
      {canManage ? (
        <SegmentedControl size="md" options={AssistOptions} value={value} onChange={(v) => void change(v)} ariaLabel="AI assist" />
      ) : (
        <Mono size="md">{current.aiAssistEnabled ? WorkspaceText.assistOn : WorkspaceText.assistOff}</Mono>
      )}
      <span className={styles.hint}>{WorkspaceText.assistHint}</span>
    </SettingRow>
  );
}

/** Continuous SIEM export: sink, health and backlog (workspace admins). */
function AuditExportRow() {
  const { data } = useServiceQuery(() => services.audit.exportStatus(), []);
  if (!data) return null;
  const view = AuditExportView.of(data);
  return (
    <SettingRow label="Audit export">
      <ToneBadge tone={view.tone}>{view.label}</ToneBadge>
      <span className={styles.hint}>{view.detail}</span>
    </SettingRow>
  );
}
