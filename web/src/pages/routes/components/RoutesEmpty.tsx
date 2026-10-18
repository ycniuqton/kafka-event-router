import { Button, EmptyState } from '../../../components/ui';
import { Can } from '../../../access';
import { Permission } from '../../../domain';
import { actions } from '../../../store';

/** Spec §12 empty state: explains what a route is and offers "+ New route". */
export function RoutesEmpty({ hasRoutes }: { hasRoutes: boolean }) {
  return (
    <EmptyState
      title={hasRoutes ? 'No routes match' : 'No routes yet'}
      description="A route moves events from one topic to another, filtered and reshaped."
      action={
        <Can permission={Permission.RouteWrite}>
          <Button variant="primary" size="lg" onClick={() => actions.openNewRouteChooser()}>
            + New route
          </Button>
        </Can>
      }
    />
  );
}
