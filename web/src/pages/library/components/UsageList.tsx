import { Link } from 'react-router-dom';
import { services } from '../../../api';
import { EmptyState, Mono, ToneBadge } from '../../../components/ui';
import { AppPaths } from '../../../app/paths';
import { LibraryRules, Tone, type LibraryKind } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { LibraryText } from '../model/LibraryLayout';
import styles from './LibraryList.module.css';

/** Routes referencing an object: pinned versions and whether the pin is in the draft or the deployed version. */
export function UsageList({ kind, id }: { kind: LibraryKind; id: string }) {
  const { data: usages, error } = useServiceQuery(() => services.library.usages(kind, id), [kind, id]);
  if (error) return <div className={styles.error}>{error.message}</div>;
  if (usages && !usages.length) return <EmptyState compact title={LibraryText.noUsages} />;
  return (
    <div className={styles.list} aria-label="Usages">
      {(usages ?? []).map((u) => (
        <div key={u.routeId} className={styles.itemHead + ' ' + styles.item}>
          <Link to={AppPaths.routeDetail(u.routeId)}>
            <Mono size="md" medium>{u.routeName}</Mono>
          </Link>
          <Mono tone={Tone.Accent}>{u.versions.map((v) => LibraryRules.versionLabel(v)).join(', ')}</Mono>
          {u.inActive && <ToneBadge tone={Tone.Ok}>{LibraryText.active}</ToneBadge>}
          {u.inDraft && <ToneBadge tone={Tone.Info}>{LibraryText.draft}</ToneBadge>}
        </div>
      ))}
    </div>
  );
}
