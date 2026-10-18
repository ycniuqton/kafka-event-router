import { SafeStorage } from '../../../store/SafeStorage';
import { RoutesView } from './RouteListModel';

/** The viewer's table / cards choice of the routes list, remembered in local storage (memory when unavailable). */
export class RoutesViewPreference {
  static readonly key = 'er.routes.view';

  static load(): RoutesView {
    const stored = SafeStorage.get(RoutesViewPreference.key);
    return stored === RoutesView.Cards ? RoutesView.Cards : RoutesView.Table;
  }

  static save(view: RoutesView): void {
    SafeStorage.set(RoutesViewPreference.key, view);
  }
}
