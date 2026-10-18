import type { ReactNode } from 'react';
import { Button, DataTable } from '../../../components/ui';
import type { DeadLetterRouteNames } from '../model/DeadLetterBrowserModel';
import type { DeadLetterBrowserModel } from '../model/useDeadLetterBrowser';
import { DeadLetterRow } from './DeadLetterRow';
import styles from './DeadLetterList.module.css';

/** Checkbox, time, error type, message + route / step / partition / offset (design dead-letter list). */
const DeadLetterColumns = '16px 64px 110px minmax(0,1fr)';

export type DeadLetterListProps = { model: DeadLetterBrowserModel; names: DeadLetterRouteNames; toolbar: ReactNode; empty: ReactNode };

/** Failed events newest first, with checkbox selection and "Load more" over the server cursor. */
export function DeadLetterList({ model, names, toolbar, empty }: DeadLetterListProps) {
  const { state } = model;
  const selectedId = state.selected?.id;
  return (
    <DataTable columns={DeadLetterColumns}>
      {toolbar}
      {state.items.map((d) => (
        <DeadLetterRow
          key={d.id}
          deadLetter={d}
          routeName={names.of(d.routeId, d.routeName)}
          selected={d.id === selectedId}
          checked={state.checked.includes(d.id)}
          onSelect={() => model.select(d.id)}
          onToggle={() => model.toggle(d.id)}
        />
      ))}
      {!state.items.length && empty}
      <DeadLetterListFooter model={model} />
    </DataTable>
  );
}

/** Loading line, scan-budget note and the "Load more" button. */
function DeadLetterListFooter({ model }: { model: DeadLetterBrowserModel }) {
  const { state } = model;
  if (state.loading && !state.items.length) return <div className={styles.note}>Reading the dead-letter topics…</div>;
  if (!state.hasMore && !state.truncated) return null;
  return (
    <div className={styles.footer}>
      {state.truncated && <span className={styles.note}>The scan stopped at its time budget; load more to keep reading older events.</span>}
      {state.hasMore && (
        <Button size="sm" disabled={state.loading} onClick={model.loadMore}>
          {state.loading ? 'Loading…' : 'Load more'}
        </Button>
      )}
    </div>
  );
}
