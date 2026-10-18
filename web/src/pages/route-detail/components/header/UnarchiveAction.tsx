import { Can } from '../../../../access';
import { Button } from '../../../../components/ui';
import { Permission, RouteStatus, type Route } from '../../../../domain';
import { actions } from '../../../../store';

/** Header "Unarchive" on archived routes: brings the route back as paused. */
export function UnarchiveAction({ route }: { route: Route }) {
  if (route.status !== RouteStatus.Archived) return null;
  return (
    <Can permission={Permission.RouteWrite}>
      <Button onClick={() => void actions.routes.unarchive(route.id)}>Unarchive</Button>
    </Can>
  );
}
