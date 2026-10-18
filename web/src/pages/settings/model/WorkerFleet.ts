import { Format } from '../../../components/ui/Format';
import { WorkerHealthMeta, type Tone, type WorkerInfo } from '../../../domain';

/** Texts of the workers section. */
export const WorkerText = {
  title: 'Workers',
  hint: 'Data-plane workers that run routes. Only workspace admins see this list.',
  empty: 'No worker has sent a heartbeat yet.',
  unavailable: 'The worker list is unavailable.',
  loading: 'Loading workers…',
  route: 'route',
  routes: 'routes',
  capacity: 'capacity',
  separator: ' · ',
} as const;

/** One worker row. */
export type WorkerRow = { id: string; place: string; label: string; tone: Tone; load: string; heartbeat: string };

/** Turns `GET /workers` into rows: live workers first, then by id; the host is omitted when it repeats the id. */
export class WorkerFleet {
  static readonly columns = '1.4fr 0.8fr 120px 1.5fr 0.8fr';
  static readonly messageColumns = '1fr';

  static rows(workers: WorkerInfo[], now: Date): WorkerRow[] {
    return [...workers].sort(WorkerFleet.order).map((w) => ({
      id: w.id,
      place: [w.host === w.id ? '' : w.host, w.zone].filter(Boolean).join(WorkerText.separator),
      ...WorkerFleet.health(w),
      load: [`${w.routes} ${w.routes === 1 ? WorkerText.route : WorkerText.routes}`, `${Format.number(w.capacityUsed)}/${w.capacity} ${WorkerText.capacity}`].join(WorkerText.separator),
      heartbeat: Format.relativeTime(w.lastHeartbeatAt, now),
    }));
  }

  private static health(w: WorkerInfo): { label: string; tone: Tone } {
    if (!w.alive) return WorkerHealthMeta.dead;
    return w.draining ? WorkerHealthMeta.draining : WorkerHealthMeta.alive;
  }

  private static order(a: WorkerInfo, b: WorkerInfo): number {
    if (a.alive !== b.alive) return a.alive ? -1 : 1;
    return a.id.localeCompare(b.id);
  }
}
