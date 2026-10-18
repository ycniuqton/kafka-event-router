import {
  ConnectionInUseError,
  ConnectionStatus,
  TopicDefaults,
  type Connection,
  type ConnectionDraft,
  type ConnectionTestRun,
  type TopicInfo,
  type TopicQuery,
  type TopicSettings,
} from '../../domain';
import { NEW_CONNECTION_TOPICS, NEW_TOPIC_DEFAULTS } from '../../mocks';
import { MockLatency } from '../config';
import { ConnectionsService } from '../contracts';
import type { MockBackend } from './MockBackend';
import { ConnectionTestSimulator } from './ConnectionTestSimulator';

/** Placeholder values the design uses when a new connection omits them. */
const NewConnectionFallback = { id: 'new-connection', server: 'localhost:9092', inUse: 'Routes still use this connection.' } as const;

/** In-memory connections backend with a timed, scripted connection test. */
export class MockConnectionsService extends ConnectionsService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(): Promise<Connection[]> {
    return this.backend.reply(this.backend.db.connections);
  }

  get(id: string): Promise<Connection> {
    return this.backend.reply(this.require(id));
  }

  create(draft: ConnectionDraft): Promise<Connection> {
    const connection: Connection = {
      ...this.fromDraft(draft, draft.name || NewConnectionFallback.id),
      status: ConnectionStatus.Untested,
      checkedAt: null,
      topics: structuredClone(NEW_CONNECTION_TOPICS),
    };
    this.backend.db.connections = [...this.backend.db.connections, connection];
    return this.backend.reply(connection);
  }

  update(id: string, draft: ConnectionDraft): Promise<Connection> {
    const current = this.require(id);
    const updated: Connection = { ...current, ...this.fromDraft(draft, id), name: current.name };
    this.backend.db.connections = this.backend.db.connections.map((c) => (c.id === id ? updated : c));
    return this.backend.reply(updated);
  }

  async remove(id: string): Promise<void> {
    const routes = this.backend.db.routes.filter((r) => r.source.connectionId === id || r.destination.connectionId === id);
    if (routes.length) throw new ConnectionInUseError(NewConnectionFallback.inUse, routes.map((r) => ({ id: r.id, name: r.name })));
    this.backend.db.connections = this.backend.db.connections.filter((c) => c.id !== id);
    await this.backend.reply(null);
  }

  test(id: string, onUpdate: (run: ConnectionTestRun) => void): Promise<ConnectionTestRun> {
    return this.simulate(this.require(id), onUpdate, () => this.markHealthy(id));
  }

  testDraft(draft: ConnectionDraft, connectionId: string | null, onUpdate: (run: ConnectionTestRun) => void): Promise<ConnectionTestRun> {
    const connection: Connection = { ...this.fromDraft(draft, connectionId ?? draft.name), status: ConnectionStatus.Untested, checkedAt: null, topics: [] };
    return this.simulate(connection, onUpdate, () => undefined);
  }

  listTopics(id: string, query: TopicQuery = {}): Promise<TopicInfo[]> {
    const q = query.q?.toLowerCase() ?? '';
    return this.backend.reply(this.require(id).topics.filter((t) => t.name.toLowerCase().includes(q)));
  }

  createTopic(id: string, name: string, settings: TopicSettings = {}): Promise<TopicInfo> {
    const topic: TopicInfo = { name, partitions: settings.partitions ?? TopicDefaults.partitions, rate: NEW_TOPIC_DEFAULTS.rate };
    this.backend.db.connections = this.backend.db.connections.map((c) => (c.id === id ? { ...c, topics: [...c.topics, topic] } : c));
    return this.backend.reply(topic);
  }

  private fromDraft(draft: ConnectionDraft, id: string): Omit<Connection, 'status' | 'checkedAt' | 'topics'> {
    return {
      id,
      env: draft.env,
      bootstrapServers: draft.bootstrapServers.length ? draft.bootstrapServers : [NewConnectionFallback.server],
      security: draft.security,
      owner: draft.team,
      description: draft.description,
      allowTopicCreate: draft.allowTopicCreate,
      probeEnabled: draft.probeEnabled,
      properties: draft.properties,
    };
  }

  private simulate(connection: Connection, onUpdate: (run: ConnectionTestRun) => void, onOk: () => void): Promise<ConnectionTestRun> {
    const simulator = new ConnectionTestSimulator(connection);
    return new Promise((resolve) => {
      let progress = 0;
      onUpdate(simulator.snapshot(progress));
      const timer = setInterval(() => {
        progress += 1;
        const run = simulator.snapshot(progress);
        onUpdate(run);
        if (!run.finished) return;
        clearInterval(timer);
        if (run.ok) onOk();
        resolve(run);
      }, MockLatency.connectionTestStepMs);
    });
  }

  private require(id: string): Connection {
    const connection = this.backend.db.connections.find((c) => c.id === id);
    if (!connection) throw new Error(`Connection ${id} not found`);
    return connection;
  }

  private markHealthy(id: string): void {
    const checkedAt = this.backend.nowIso();
    const db = this.backend.db;
    db.connections = db.connections.map((c) => (c.id === id ? { ...c, status: ConnectionStatus.Healthy, checkedAt } : c));
  }
}
