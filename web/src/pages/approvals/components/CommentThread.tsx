import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, Card, Format, TextArea } from '../../../components/ui';
import { ApprovalLimits, ApprovalRules, type ApprovalComment } from '../../../domain';
import { actions, useNow } from '../../../store';
import { ReviewText } from '../model/DecisionModel';
import styles from '../ApprovalReviewPage.module.css';

export type CommentThreadProps = { approvalId: string; comments: ApprovalComment[]; onPosted: () => void };

/** Review discussion: anyone who can read the route may comment (1–4000 characters). */
export function CommentThread({ approvalId, comments, onPosted }: CommentThreadProps) {
  const [body, setBody] = useState('');
  const now = useNow();
  const post = async () => {
    try {
      await services.approvals.comment(approvalId, body);
      actions.toast.show(ReviewText.Commented);
      setBody('');
      onPosted();
    } catch (error) {
      actions.toast.show((error as Error).message);
    }
  };
  return (
    <Card padding="md" className={styles.thread}>
      <div className={styles.cardTitle}>Comments</div>
      {comments.length === 0 && <div className={styles.muted}>No comments yet.</div>}
      {comments.map((c) => (
        <div key={c.id} className={styles.comment}>
          <span className={styles.author}>{c.author?.name ?? 'Former member'}</span>
          <span className={styles.muted}>{Format.relativeTime(c.createdAt, now)}</span>
          <div className={styles.body}>{c.body}</div>
        </div>
      ))}
      <TextArea value={body} rows={2} maxLength={ApprovalLimits.maxComment} placeholder="Add a comment" aria-label="Comment" onChange={(e) => setBody(e.target.value)} />
      <div className={styles.buttons}>
        <Button size={ButtonSize.Sm} disabled={!ApprovalRules.validComment(body)} onClick={() => void post()}>
          Comment
        </Button>
      </div>
    </Card>
  );
}
