import type {
  Activity,
  Alert,
  Connection,
  ConnectionTestRun,
  CurrentUser,
  DeadLetterJob,
  DeadLetterSummary,
  Route,
  RouteVersion,
} from '../domain';
import { DeadLetterSummaries, PushState } from '../domain';
import { ExternalStore } from './ExternalStore';

export type ToastMessage = { id: number; message: string };

/** Shared, mutable state that several screens read and change. */
export type AppState = {
  ready: boolean;
  routes: Route[];
  connections: Connection[];
  connectionTests: Record<string, ConnectionTestRun>;
  deadLetterSummary: DeadLetterSummary;
  deadLetterJobs: DeadLetterJob[];
  activity: Activity[];
  versions: Record<string, RouteVersion[]>;
  alerts: Alert[];
  /** Bell badge: alerts the user has not acknowledged (monitoring). */
  alertsUnread: number;
  /** State of the `WS /events` stream and a counter bumped by every push frame that changes shared data. */
  pushState: PushState;
  pushSeq: number;
  currentUser: CurrentUser | null;
  toast: ToastMessage | null;
  newRouteChooserOpen: boolean;
};

const initialState: AppState = {
  ready: false,
  routes: [],
  connections: [],
  connectionTests: {},
  deadLetterSummary: DeadLetterSummaries.empty,
  deadLetterJobs: [],
  activity: [],
  versions: {},
  alerts: [],
  alertsUnread: 0,
  pushState: PushState.Offline,
  pushSeq: 0,
  currentUser: null,
  toast: null,
  newRouteChooserOpen: false,
};

/** The single application store instance type. */
export class AppStore extends ExternalStore<AppState> {
  constructor() {
    super(initialState);
  }
}
