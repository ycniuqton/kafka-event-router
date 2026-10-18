import { Button, Format, ToneBadge } from '../../../components/ui';
import { Can } from '../../../access';
import { Permission, Tone, type RouteVersion } from '../../../domain';
import { VersionPresentation } from '../model/VersionPresentation';
import styles from './VersionRow.module.css';

type VersionRowProps = {
  version: RouteVersion;
  current: boolean;
  selected: boolean;
  compared: boolean;
  onSelect: () => void;
  onCompare: () => void;
  onRollback: () => void;
};

/** A deployed version: compare tick, number, note, origin, author and time, deployment state, running tag or Roll back. */
export function VersionRow({ version, current, selected, compared, onSelect, onCompare, onRollback }: VersionRowProps) {
  const origin = VersionPresentation.origin(version);
  const status = VersionPresentation.status(version);
  return (
    <div className={selected ? `${styles.row} ${styles.selected}` : styles.row} onClick={onSelect}>
      <input
        type="checkbox"
        className={styles.check}
        checked={compared}
        onClick={(e) => e.stopPropagation()}
        onChange={onCompare}
        aria-label={`Compare v${version.version}`}
        title="Select two versions to compare"
      />
      <span className={styles.label}>v{version.version}</span>
      <div className={styles.body}>
        <div className={styles.note}>{version.note}</div>
        <div className={styles.meta}>
          {origin && <span className={styles.origin}>{origin} · </span>}
          {version.author} · {Format.versionDate(version.createdAt)}
        </div>
      </div>
      {status && (
        <ToneBadge tone={status.tone} dot={false}>
          {status.label}
        </ToneBadge>
      )}
      {current ? (
        <ToneBadge tone={Tone.Ok} dot={false}>
          {VersionPresentation.runningLabel}
        </ToneBadge>
      ) : (
        <Can permission={Permission.RouteDeployNonProd}>
          <Button
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              onRollback();
            }}
          >
            Roll back
          </Button>
        </Can>
      )}
    </div>
  );
}
