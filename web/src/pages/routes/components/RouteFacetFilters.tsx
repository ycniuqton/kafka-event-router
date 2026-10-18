import { Select } from '../../../components/ui';
import type { Route } from '../../../domain';
import { AnyOption, RouteFacets } from '../model/RouteListModel';
import styles from './RoutesToolbar.module.css';

type RouteFacetFiltersProps = {
  routes: Route[];
  connectionId: string;
  onConnection: (id: string) => void;
  tag: string;
  onTag: (tag: string) => void;
  owner: string;
  onOwner: (owner: string) => void;
};

/** Owner team, connection and tag dropdowns of the toolbar; options come from the loaded routes (names, not ids). */
export function RouteFacetFilters({ routes, connectionId, onConnection, tag, onTag, owner, onOwner }: RouteFacetFiltersProps) {
  const connections = RouteFacets.connections(routes);
  const tags = RouteFacets.tags(routes);
  const owners = RouteFacets.owners(routes);
  return (
    <>
      <Select className={styles.facet} value={owner} onChange={(e) => onOwner(e.target.value)} aria-label="Filter by owner team">
        <option value={AnyOption}>All teams</option>
        {owners.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
        {owner !== AnyOption && !owners.includes(owner) && <option value={owner}>{owner}</option>}
      </Select>
      <Select className={styles.facet} value={connectionId} onChange={(e) => onConnection(e.target.value)} aria-label="Filter by connection">
        <option value={AnyOption}>All connections</option>
        {connections.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </Select>
      <Select
        className={styles.facet}
        value={tag}
        onChange={(e) => onTag(e.target.value)}
        aria-label="Filter by tag"
        disabled={!tags.length && tag === AnyOption}
      >
        <option value={AnyOption}>{tags.length ? 'All tags' : 'No tags'}</option>
        {tags.map((t) => (
          <option key={t} value={t}>
            #{t}
          </option>
        ))}
        {tag !== AnyOption && !tags.includes(tag) && <option value={tag}>#{tag}</option>}
      </Select>
    </>
  );
}
