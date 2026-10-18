import { Page, PageHeader, PageWidth } from '../../components/ui';
import styles from './NotFoundPage.module.css';

/** Page not found screen — placeholder rendered until the screen is implemented. */
export default function NotFoundPage() {
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <PageHeader title="Page not found" subtitle="This page does not exist." />
      </Page>
    </div>
  );
}
