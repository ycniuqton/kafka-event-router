import { services } from '../../../api';
import { ReplayRules, ReplayStatus, type Replay } from '../../../domain';
import { actions } from '../../../store';
import { ReplayText } from './ReplayForm';

/** Toasts "Replay started", then polls the replay until it is done or failed and toasts the result. */
export class ReplayTracker {
  static readonly maxPolls = 90;

  static follow(replay: Replay, destination: string): void {
    actions.toast.show(ReplayText.started(destination));
    ReplayTracker.poll(replay, 0);
  }

  private static poll(replay: Replay, attempt: number): void {
    if (ReplayRules.finished(replay.status)) {
      actions.toast.show(replay.status === ReplayStatus.Failed ? ReplayText.failed(replay.error) : ReplayText.done);
      return;
    }
    if (attempt >= ReplayTracker.maxPolls) return;
    setTimeout(() => {
      void services.routes
        .replayStatus(replay.routeId, replay.id)
        .then((next) => ReplayTracker.poll(next, attempt + 1))
        .catch(() => undefined);
    }, ReplayRules.pollIntervalMs);
  }
}
