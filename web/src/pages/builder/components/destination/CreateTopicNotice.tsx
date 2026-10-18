import { useState } from 'react';
import { Can } from '../../../../access';
import { Button } from '../../../../components/ui';
import { Permission } from '../../../../domain';
import { DestinationRules } from '../../model/ReviewModel';
import styles from './CreateTopicNotice.module.css';

export type CreateTopicNoticeProps = {
  topic: string;
  allowed: boolean;
  onDeploy: boolean;
  onToggleDeploy: (on: boolean) => void;
  onCreate: () => Promise<void>;
};

/** Warning when the destination topic does not exist: create it now, or let the deploy create it. */
export function CreateTopicNotice({ topic, allowed, onDeploy, onToggleDeploy, onCreate }: CreateTopicNoticeProps) {
  const [busy, setBusy] = useState(false);
  const create = () => {
    setBusy(true);
    void onCreate().finally(() => setBusy(false));
  };
  return (
    <div className={styles.notice}>
      <span className={styles.text}>
        Topic <b className={styles.mono}>{topic}</b> doesn't exist on this connection.
        {!allowed && ' Topic creation is turned off for this connection.'}
      </span>
      {allowed && (
        <Can permission={Permission.TopicCreate}>
          <label className={styles.toggle}>
            <input type="checkbox" checked={onDeploy} onChange={(e) => onToggleDeploy(e.target.checked)} />
            Create on deploy
          </label>
          <Button className={styles.create} disabled={busy} onClick={create}>
            {busy ? 'Creating…' : `Create now · ${DestinationRules.newTopicPartitions} partitions`}
          </Button>
        </Can>
      )}
    </div>
  );
}
