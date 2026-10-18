import { services } from '../../../api';
import type { Connection, RouteDraft, TopicInfo } from '../../../domain';
import { useConnections, useServiceQuery } from '../../../store';

/**
 * Fills topics of the draft's source and destination connections from `/connections/{id}/topics`.
 * Re-reads whenever the stored connections change (e.g. after "Create topic"), so new topics show up.
 */
export class DraftConnectionTopics {
  static async load(connections: Connection[], ids: string[]): Promise<Connection[]> {
    const wanted = [...new Set(ids)].filter((id) => connections.some((c) => c.id === id));
    const loaded = await Promise.all(wanted.map(async (id) => [id, await DraftConnectionTopics.topics(id)] as const));
    const topics = new Map<string, TopicInfo[] | null>(loaded);
    return connections.map((c) => ({ ...c, topics: topics.get(c.id) ?? c.topics }));
  }

  static key(draft: RouteDraft): string[] {
    return [draft.source.connectionId, draft.destination.connectionId];
  }

  /** Topics of one connection, or null when the cluster cannot be listed (the stored list is kept). */
  private static async topics(id: string): Promise<TopicInfo[] | null> {
    try {
      return await services.connections.listTopics(id);
    } catch {
      return null;
    }
  }
}

/** The builder's connection list with topics browsable for the connections the draft uses. */
export function useBuilderConnections(draft: RouteDraft): Connection[] {
  const connections = useConnections();
  const ids = DraftConnectionTopics.key(draft);
  const { data } = useServiceQuery(() => DraftConnectionTopics.load(connections, ids), [connections, ...ids]);
  return data ?? connections;
}
