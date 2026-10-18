import { Link } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { useServiceQuery } from '../../../store';
import { ConnectionGrants, GrantsText } from '../model/ConnectionGrants';
import styles from './GrantedTeams.module.css';

export type GrantedTeamsProps = { connectionId: string };

/** Read-only list of teams granted this connection, with a link to the grants editor in Settings (CN-112). */
export function GrantedTeams({ connectionId }: GrantedTeamsProps) {
  const { data, loading } = useServiceQuery(() => ConnectionGrants.current().grantedTeams(connectionId), [connectionId]);
  const note = data === null || data === undefined ? GrantsText.unavailable : GrantsText.none;
  return (
    <div className={styles.grants}>
      <span className={styles.label}>{GrantsText.label}</span>
      {loading && <span className={styles.note}>{GrantsText.loading}</span>}
      {!loading && data?.length ? (
        <ul className={styles.teams}>
          {data.map((t) => (
            <li key={t.teamId} className={styles.team}>
              {t.teamName}
            </li>
          ))}
        </ul>
      ) : (
        !loading && <span className={styles.note}>{note}</span>
      )}
      <Link className={styles.link} to={AppPaths.settings()}>
        {GrantsText.manage}
      </Link>
    </div>
  );
}
