import { services } from '../../api';
import { Page, PageHeader, PageWidth } from '../../components/ui';
import { useServiceQuery } from '../../store';
import { MembersSection } from './components/MembersSection';
import { NotificationChannelsSection } from './components/NotificationChannelsSection';
import { RolesSection } from './components/RolesSection';
import { TeamsSection } from './components/TeamsSection';
import { WorkersSection } from './components/WorkersSection';
import { WorkspaceSection } from './components/WorkspaceSection';
import { ApiTokensPanel } from './ApiTokensPanel';
import styles from './SettingsPage.module.css';

/** Settings: workspace preferences, members & roles, teams and notification channels. */
export default function SettingsPage() {
  const { data: members, reload } = useServiceQuery(() => services.access.members(), []);
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Narrow} relaxed>
        <PageHeader title="Settings" />
        <WorkspaceSection />
        <MembersSection members={members ?? []} onChanged={reload} />
        <RolesSection />
        <TeamsSection onChanged={reload} />
        <NotificationChannelsSection />
        <ApiTokensPanel />
        <WorkersSection />
      </Page>
    </div>
  );
}
