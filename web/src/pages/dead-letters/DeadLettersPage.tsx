import { Page, PageHeader, PageWidth } from '../../components/ui';
import { DeadLetterAuditLink } from './components/DeadLetterAuditLink';
import { ReplayApprovalsPanel } from './components/ReplayApprovalsPanel';
import { DeadLetterBrowser } from './DeadLetterBrowser';
import styles from './DeadLettersPage.module.css';

/** Dead letters (spec §10.3): failed events of every readable route, filterable by `?route=&type=&range=`. */
export default function DeadLettersPage() {
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <PageHeader title="Dead letters" subtitle="Events that failed, kept with the reason." actions={<DeadLetterAuditLink />} />
        <ReplayApprovalsPanel />
        <DeadLetterBrowser />
      </Page>
    </div>
  );
}
