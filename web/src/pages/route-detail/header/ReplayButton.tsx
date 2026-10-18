import { useState } from 'react';
import { services } from '../../../api';
import { Button } from '../../../components/ui';
import { RouteEndpointText, RouteStatus, type Replay, type ReplayRequest, type Route } from '../../../domain';
import { ReplayDialog } from './ReplayDialog';
import { ReplayTracker } from './ReplayTracker';

/** "Replay" header action: opens the confirm dialog and follows the started replay with toasts. */
export function ReplayButton({ route }: { route: Route }) {
  const [open, setOpen] = useState(false);
  if (route.status === RouteStatus.Draft || route.status === RouteStatus.Archived) return null;
  const start = (request: ReplayRequest) => services.routes.replay(route.id, request);
  const started = (replay: Replay) => {
    setOpen(false);
    ReplayTracker.follow(replay, RouteEndpointText.destination(route));
  };
  return (
    <>
      <Button onClick={() => setOpen(true)}>Replay</Button>
      {open && <ReplayDialog route={route} onClose={() => setOpen(false)} onStarted={started} start={start} />}
    </>
  );
}
