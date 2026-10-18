import { Button, ButtonSize, ButtonVariant, StatusBadge } from '../../../components/ui';
import { Can } from '../../../access';
import { Permission, type RouteStatus } from '../../../domain';
import styles from './BuilderTopBar.module.css';

export type BuilderTopBarProps = {
  name: string;
  status: RouteStatus;
  dirty: boolean;
  onRename: (name: string) => void;
  onBack: () => void;
  onSave: () => void;
  onTest: () => void;
  onDeploy: () => void;
};

/** Builder header: back, inline-editable name, status, unsaved indicator, Save draft, Test, Deploy…. */
export function BuilderTopBar({ name, status, dirty, onRename, onBack, onSave, onTest, onDeploy }: BuilderTopBarProps) {
  return (
    <div className={styles.bar}>
      <button type="button" className={styles.back} onClick={onBack}>
        ← Back
      </button>
      <input className={styles.name} value={name} onChange={(e) => onRename(e.target.value)} aria-label="Route name" />
      <StatusBadge status={status} />
      <div className={styles.spacer} />
      {dirty && <span className={styles.unsaved}>● Unsaved changes</span>}
      <Button title="Ctrl+S" onClick={onSave}>
        Save draft
      </Button>
      <Button title="Ctrl+Enter" onClick={onTest}>
        Test
      </Button>
      <Can permission={Permission.RouteDeployNonProd}>
        <Button variant={ButtonVariant.Primary} size={ButtonSize.Md} className={styles.deploy} onClick={onDeploy}>
          Deploy…
        </Button>
      </Can>
    </div>
  );
}
