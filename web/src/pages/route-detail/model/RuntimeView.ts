import { Format } from '../../../components/ui/Format';
import { HaltReasonLabel, ObservedStateMeta, RunnerStateMeta, type RouteRuntime, type Tone } from '../../../domain';

/** Texts of the runtime card. */
export const RuntimeText = {
  title: 'Runtime',
  noVersion: 'No version running',
  noReplicas: 'No worker runs this route right now.',
  unavailable: 'Runtime status is unavailable.',
  loading: 'Loading runtime…',
  retry: 'Retry',
  halted: 'Halted',
  running: 'Running v',
  lag: 'lag',
  replica: 'replica',
  replicas: 'replicas',
  noValue: '—',
  separator: ' · ',
  columns: ['Worker', 'State', 'Version', 'Lag', 'Last report'],
} as const;

/** One replica row of the runtime card. */
export type ReplicaRow = { worker: string; label: string; tone: Tone; version: string; lag: string; reported: string; error: string | null };

/** Halt banner content (`haltedAt` set). */
export type HaltBanner = { reason: string; message: string | null; since: string };

/** Turns `GET /routes/{id}/runtime` into card texts and rows. */
export class RuntimeView {
  static readonly columns = '1.6fr 1fr 70px 80px 1fr';

  static state(rt: RouteRuntime): { label: string; tone: Tone } {
    return ObservedStateMeta[rt.observedState];
  }

  static meta(rt: RouteRuntime): string {
    const version = rt.runningVersionNo === null ? RuntimeText.noVersion : `${RuntimeText.running}${rt.runningVersionNo}`;
    const replicas = `${rt.replicas.length} ${rt.replicas.length === 1 ? RuntimeText.replica : RuntimeText.replicas}`;
    return [version, `${RuntimeText.lag} ${Format.number(rt.lag)}`, replicas].join(RuntimeText.separator);
  }

  static halt(rt: RouteRuntime, now: Date): HaltBanner | null {
    if (!rt.haltedAt) return null;
    const reason = rt.haltReason ? HaltReasonLabel[rt.haltReason] : RuntimeText.halted;
    return { reason, message: rt.errorMessage, since: Format.relativeTime(rt.haltedAt, now) };
  }

  static rows(rt: RouteRuntime, now: Date): ReplicaRow[] {
    return rt.replicas.map((r) => ({
      worker: r.workerId,
      ...RunnerStateMeta[r.state],
      version: r.versionNo === null ? RuntimeText.noValue : `v${r.versionNo}`,
      lag: Format.number(r.lag),
      reported: Format.relativeTime(r.reportedAt, now),
      error: r.errorMessage,
    }));
  }
}
