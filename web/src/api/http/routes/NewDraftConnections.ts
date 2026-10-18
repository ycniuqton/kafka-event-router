import type { RouteDraft } from '../../../domain';
import { ConnectionEndpoint, type ConnectionDto } from '../connections/ConnectionDtos';
import { CursorIterator } from '../core/CursorIterator';
import type { HttpContext } from '../core/HttpContext';

/** Lookup size: one visible connection is enough to seed a new draft. */
export const NewDraftSeed = { pageLimit: 1, none: '' } as const;

/**
 * Points a brand-new draft at a real connection. The shared defaults (`orders-prod`, `order-events`, …) are mock
 * names that do not exist over REST: source and destination start on the first visible connection and the
 * source topics start empty, so the user picks real ones (the rail shows "Pick a topic").
 */
export class NewDraftConnections {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    this.context = context;
  }

  async apply(draft: RouteDraft): Promise<RouteDraft> {
    const pages = new CursorIterator<ConnectionDto>(this.context.api, ConnectionEndpoint.collection, { limit: NewDraftSeed.pageLimit });
    const [first] = await pages.nextPage();
    const id = first?.id ?? NewDraftSeed.none;
    return {
      ...draft,
      source: { ...draft.source, connectionId: id, topics: [] },
      destination: { ...draft.destination, connectionId: id },
    };
  }
}
