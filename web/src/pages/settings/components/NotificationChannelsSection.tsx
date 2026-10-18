import { Button, Format, ToneBadge } from '../../../components/ui';
import { actions } from '../../../store';
import { ChannelStatus, NotificationChannelCatalog } from '../model/SettingsCatalog';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import styles from './SettingsSection.module.css';

const ChannelColumns = '140px minmax(0,1fr) 110px 100px';

/** Where alerts are delivered: email, Slack, Microsoft Teams, webhook (spec §11.4). */
export function NotificationChannelsSection() {
  return (
    <SettingsSection title="Notification channels" hint="Alerts also appear in the bell menu and on the dashboard.">
      {NotificationChannelCatalog.channels.map((c) => {
        const status = c.target ? ChannelStatus.connected : ChannelStatus.notSetUp;
        return (
          <SettingsGridRow key={c.kind} columns={ChannelColumns}>
            <span>{c.name}</span>
            <span className={styles.mono}>{c.target ?? Format.placeholder}</span>
            <ToneBadge tone={status.tone}>{status.label}</ToneBadge>
            <span className={styles.actions}>
              <Button
                size="sm"
                disabled={!c.target}
                title={c.target ? undefined : status.label}
                onClick={() => actions.toast.show(NotificationChannelCatalog.testSentPrefix + c.target)}
              >
                Send test
              </Button>
            </span>
          </SettingsGridRow>
        );
      })}
    </SettingsSection>
  );
}
