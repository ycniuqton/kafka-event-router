import { Can } from '../../../access';
import { Button } from '../../../components/ui';
import { Permission } from '../../../domain';
import type { BulkRouteActions } from '../model/BulkRouteActions';
import styles from './BulkActionBar.module.css';

type BulkActionBarProps = { count: number; bulk: BulkRouteActions; onAddTag: () => void };

/** "N selected" with Pause / Resume / Add tag / Export YAML for the checked routes. */
export function BulkActionBar({ count, bulk, onAddTag }: BulkActionBarProps) {
  return (
    <div className={styles.bar}>
      <span>{count} selected</span>
      <Can permission={Permission.RouteWrite}>
        <Button size="sm" className={styles.action} onClick={() => bulk.pause()}>Pause</Button>
        <Button size="sm" className={styles.action} onClick={() => bulk.resume()}>Resume</Button>
        <Button size="sm" className={styles.action} onClick={onAddTag}>Add tag</Button>
      </Can>
      <Button size="sm" className={styles.action} onClick={() => bulk.exportYaml()}>Export YAML</Button>
      <Button size="sm" variant="ghost" onClick={() => bulk.clear()} aria-label="Clear selection">×</Button>
    </div>
  );
}
