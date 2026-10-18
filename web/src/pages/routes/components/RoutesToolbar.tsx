import type { ReactNode } from 'react';
import { Can } from '../../../access';
import { Button, Chip, SegmentedControl, TextInput } from '../../../components/ui';
import { Permission, type Route } from '../../../domain';
import { RoutesView, StatusFilterOptions } from '../model/RouteListModel';
import type { RouteListParams } from '../model/useRouteListParams';
import { RouteFacetFilters } from './RouteFacetFilters';
import styles from './RoutesToolbar.module.css';

const viewOptions = [
  { value: RoutesView.Table, label: 'Table' },
  { value: RoutesView.Cards, label: 'Cards' },
] as const;

type RoutesToolbarProps = {
  routes: Route[];
  params: RouteListParams;
  view: RoutesView;
  onView: (v: RoutesView) => void;
  bulk?: ReactNode;
  onImport: () => void;
};

/** Search, owner/connection/tag filters, status chips (incl. archived), bulk actions, Import and the table/cards toggle. */
export function RoutesToolbar({ routes, counted, params, view, onView, bulk, onImport }: RoutesToolbarProps & { counted: Route[] }) {
  return (
    <div className={styles.toolbar}>
      <TextInput
        className={styles.search}
        value={params.query}
        onChange={(e) => params.setQuery(e.target.value)}
        placeholder="Filter by name, topic, connection, tag…"
        aria-label="Filter routes"
      />
      <RouteFacetFilters
        routes={routes}
        connectionId={params.connectionId}
        onConnection={params.setConnection}
        tag={params.tag}
        onTag={params.setTag}
        owner={params.owner}
        onOwner={params.setOwner}
      />
      <div className={styles.chips}>
        {StatusFilterOptions.chips.map((chip) => (
          <Chip key={chip} active={params.status === chip} count={StatusFilterOptions.count(counted, chip)} onClick={() => params.setStatus(chip)}>
            {StatusFilterOptions.label(chip)}
          </Chip>
        ))}
      </div>
      <span className={styles.spacer} />
      {bulk}
      <Can permission={Permission.RouteWrite}>
        <Button size="sm" onClick={onImport}>
          Import
        </Button>
      </Can>
      <div className={styles.view}>
        <SegmentedControl options={viewOptions} value={view} onChange={onView} ariaLabel="View" />
      </div>
    </div>
  );
}
