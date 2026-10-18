import { useNavigate } from 'react-router-dom';
import { Button } from '../../../components/ui';
import type { Connection, Route } from '../../../domain';
import { useDeadLetterSummary } from '../../../store';
import { DemoDeadLetters } from '../model/DemoDeadLetters';

export type DemoDeadLettersLinkProps = { connections: Connection[]; routes: Route[]; className?: string };

/** First-run checklist entry to the demo route's seeded dead letters; hidden without a demo route. */
export function DemoDeadLettersLink({ connections, routes, className }: DemoDeadLettersLinkProps) {
  const navigate = useNavigate();
  const summary = useDeadLetterSummary();
  const route = DemoDeadLetters.route(connections, routes);
  if (!route) return null;
  return (
    <div className={className} data-testid="demo-dead-letters">
      <span>{DemoDeadLetters.label(route, summary)}</span>
      <Button size="sm" onClick={() => navigate(DemoDeadLetters.path(route))}>
        Open dead letters
      </Button>
    </div>
  );
}
