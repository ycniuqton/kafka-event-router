import { useState } from 'react';
import type { RoutesView } from './RouteListModel';
import { RoutesViewPreference } from './RoutesViewPreference';

/** Table / cards choice of the routes list, remembered per viewer. */
export function useRoutesView(): [RoutesView, (view: RoutesView) => void] {
  const [view, setView] = useState<RoutesView>(RoutesViewPreference.load);
  const change = (next: RoutesView) => {
    setView(next);
    RoutesViewPreference.save(next);
  };
  return [view, change];
}
