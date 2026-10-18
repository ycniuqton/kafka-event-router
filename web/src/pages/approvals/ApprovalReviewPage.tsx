import { useNavigate, useParams } from 'react-router-dom';
import { services } from '../../api';
import { AppPaths } from '../../app/paths';
import { EmptyState, Page, PageHeader, PageWidth } from '../../components/ui';
import { useServiceQuery } from '../../store';
import { ApprovalDiff } from './components/ApprovalDiff';
import { ApprovalSummary } from './components/ApprovalSummary';
import { CommentThread } from './components/CommentThread';
import { DecisionBar } from './components/DecisionBar';
import styles from './ApprovalReviewPage.module.css';

/** Review page of one production deploy request (`/approvals/:id`, opened from the bell and route History). */
export default function ApprovalReviewPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const { data, error, reload } = useServiceQuery(() => services.approvals.get(id), [id]);
  const back = { label: 'Routes /', onClick: () => navigate(data ? AppPaths.routeDetail(data.approval.routeId) : AppPaths.routes()) };
  if (error) {
    return (
      <Page width={PageWidth.Narrow}>
        <PageHeader title="Deploy request" back={back} />
        <EmptyState title="This request can't be shown" description={error.message} />
      </Page>
    );
  }
  return (
    <Page width={PageWidth.Library}>
      <PageHeader title={data ? `Deploy request · ${data.approval.routeName}` : 'Deploy request'} back={back} />
      {data && (
        <div className={styles.layout}>
          <div className={styles.main}>
            <ApprovalSummary detail={data} />
            <ApprovalDiff detail={data} />
          </div>
          <div className={styles.side}>
            <DecisionBar detail={data} onDecided={reload} />
            <CommentThread approvalId={data.approval.id} comments={data.comments} onPosted={reload} />
          </div>
        </div>
      )}
    </Page>
  );
}
