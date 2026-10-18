import type { Route } from '../../../domain';
import { DeadLetterBrowser } from '../../dead-letters/DeadLetterBrowser';

/** Dead letters of this route: the shared browser fixed to the route (spec §10.3). */
export function DeadLettersTab({ route }: { route: Route }) {
  return <DeadLetterBrowser fixedRouteId={route.id} />;
}
