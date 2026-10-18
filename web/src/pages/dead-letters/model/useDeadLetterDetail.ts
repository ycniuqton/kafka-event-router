import type { DeadLetter, DeadLetterDetail } from '../../../domain';
import { actions, useServiceQuery } from '../../../store';

/** Loads the stack trace and original event of the selected dead letter. */
export function useDeadLetterDetail(deadLetter: DeadLetter): { detail: DeadLetterDetail | undefined; loading: boolean } {
  const { data, loading } = useServiceQuery(() => actions.deadLetters.detail(deadLetter), [deadLetter.id, deadLetter.routeId]);
  return { detail: data, loading };
}
