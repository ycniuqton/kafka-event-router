import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Button, Mono, PageHeader, StatusBadge } from '../../../components/ui';
import { RouteRules, Tone, type Route } from '../../../domain';
import { actions } from '../../../store';
import { HeaderActionSlot } from '../model/HeaderActionSlot';
import { RouteDetailModel } from '../model/RouteDetailModel';

/** "Routes /" breadcrumb, name, status, version · owner and Pause/Resume, Duplicate, registered actions (Replay, Export, …), Edit route. */
export function DetailHeader({ route }: { route: Route }) {
  const navigate = useNavigate();
  const model = new RouteDetailModel(route);
  const actionsNode = (
    <>
      {model.canPause && <Button onClick={() => void actions.routes.togglePause(route.id)}>{RouteRules.pauseLabel(route.status)}</Button>}
      <Button onClick={() => void actions.routes.duplicate(route.id)}>Duplicate</Button>
      {HeaderActionSlot.all().map(({ id, Component }) => (
        <Component key={id} route={route} />
      ))}
      <Button variant="primary" onClick={() => navigate(AppPaths.routeEdit(route.id))}>
        Edit route
      </Button>
    </>
  );
  return (
    <PageHeader title={route.name} back={{ label: 'Routes /', onClick: () => navigate(AppPaths.routes()) }} actions={actionsNode}>
      <StatusBadge status={route.status} />
      <Mono tone={Tone.Muted}>
        v{route.version} · {route.owner}
      </Mono>
    </PageHeader>
  );
}
