import { Button, EmptyState } from '../../../components/ui';

export type DeadLetterEmptyStateProps = { filtered: boolean; error: string | null; onClear: () => void; onRetry: () => void };

/** Empty list: nothing failed, nothing matches the filters, or the DLQ could not be read. */
export function DeadLetterEmptyState({ filtered, error, onClear, onRetry }: DeadLetterEmptyStateProps) {
  if (error) {
    return <EmptyState compact title="Dead letters could not be read." description={error} action={<Button size="sm" onClick={onRetry}>Try again</Button>} />;
  }
  if (filtered) {
    return <EmptyState compact title="No dead letters match these filters." action={<Button size="sm" onClick={onClear}>Clear filters</Button>} />;
  }
  return <EmptyState compact title="No dead letters." description="Failed events land here with the reason, so you can fix the rule and retry." />;
}
