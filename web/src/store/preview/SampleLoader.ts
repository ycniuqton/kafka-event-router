import { PreviewLimit, StopReason, type LoadSamplesRequest, type SampleRecord, type SampleSession, type ValueFormat } from '../../domain';
import type { PreviewService } from '../../api';
import { PreviewPhase } from './PreviewState';
import type { PreviewStore } from './PreviewStore';

/** Loads a sample session (all pages up to the hard limit), its schema, and serves text search. */
export class SampleLoader {
  /** Cursor value meaning "from the first page". */
  static readonly firstCursor = '';

  private static attachedSession(id: string, loaded: number): SampleSession {
    return { id, expiresAt: '', loaded, bytes: 0, truncated: false, stopReason: StopReason.Complete, masked: false };
  }

  private readonly store: PreviewStore;
  private readonly service: PreviewService;
  private generation = 0;

  constructor(store: PreviewStore, service: PreviewService) {
    this.store = store;
    this.service = service;
  }

  async load(request: LoadSamplesRequest): Promise<boolean> {
    const generation = ++this.generation;
    this.closeCurrent();
    this.store.setState({ request, loadPhase: PreviewPhase.Loading, loadFailure: null, result: null, runFailure: null, schema: null, searchHits: null, query: '' });
    try {
      const first = await this.service.loadSamples(request);
      const records = await this.rest(first.session.id, first.events, first.nextCursor);
      if (generation !== this.generation) return this.discard(first.session.id);
      this.store.setState({ session: first.session, records, loadPhase: PreviewPhase.Ready, selectedSeq: null });
      void this.loadSchema(first.session.id, generation);
      return true;
    } catch (error) {
      if (generation !== this.generation) return false;
      this.store.setState({ session: null, records: [], loadPhase: PreviewPhase.Failed, loadFailure: this.service.failure(error) });
      return false;
    }
  }

  /** Adopts an existing session (deep link `?session=`, e.g. dead letters "Open in Studio"). */
  async attach(sessionId: string): Promise<boolean> {
    const generation = ++this.generation;
    this.store.setState({ request: null, loadPhase: PreviewPhase.Loading, loadFailure: null, result: null, runFailure: null, schema: null });
    try {
      const records = await this.rest(sessionId, [], SampleLoader.firstCursor);
      if (generation !== this.generation) return false;
      const session = SampleLoader.attachedSession(sessionId, records.length);
      this.store.setState({ session, records, loadPhase: PreviewPhase.Ready, selectedSeq: null });
      void this.loadSchema(sessionId, generation);
      return true;
    } catch (error) {
      if (generation !== this.generation) return false;
      this.store.setState({ session: null, records: [], loadPhase: PreviewPhase.Failed, loadFailure: this.service.failure(error) });
      return false;
    }
  }

  /** Re-decodes the session with another value format (FR-8); runs decode with the draft format, so the last result stays. */
  async reformat(format: ValueFormat): Promise<boolean> {
    const { session, request } = this.store.getState();
    if (!session) return false;
    const generation = ++this.generation;
    this.store.setState({ reformatting: true, runFailure: null, schema: null });
    try {
      const first = await this.service.reformat(session.id, format);
      const records = await this.rest(session.id, first.events, first.nextCursor);
      if (generation !== this.generation) return false;
      this.store.setState({ session: first.session, records, request: request ? { ...request, format } : request, reformatting: false });
      void this.loadSchema(session.id, generation);
      return true;
    } catch (error) {
      if (generation === this.generation) this.store.setState({ reformatting: false, runFailure: this.service.failure(error) });
      return false;
    }
  }

  /** Re-creates the session with the last request (expired session, manual refresh). */
  reload(): Promise<boolean> {
    const request = this.store.getState().request;
    return request ? this.load(request) : Promise.resolve(false);
  }

  /** Server-side text search over key, value and headers; an empty query clears it. */
  async search(query: string): Promise<void> {
    const session = this.store.getState().session;
    this.store.setState({ query });
    if (!session || !query.trim()) return this.store.setState({ searchHits: null });
    try {
      const page = await this.service.listEvents(session.id, { q: query, limit: PreviewLimit.MaxEvents });
      if (this.store.getState().query === query) this.store.setState({ searchHits: new Set(page.events.map((e) => e.seq)) });
    } catch (error) {
      this.store.setState({ loadFailure: this.service.failure(error) });
    }
  }

  /** Forgets the current session server-side (fire and forget). */
  closeCurrent(): void {
    const session = this.store.getState().session;
    if (session) void this.service.close(session.id).catch(() => undefined);
  }

  /** Invalidates in-flight loads (unmount); the next ensure-load starts afresh. */
  cancel(): void {
    this.generation++;
    this.store.setState({ request: null, loadPhase: PreviewPhase.Idle });
  }

  /** True when `request` is already loaded or loading (effects re-running with the same source, any format). */
  isCurrent(request: LoadSamplesRequest): boolean {
    const state = this.store.getState();
    const active = state.loadPhase === PreviewPhase.Loading || state.loadPhase === PreviewPhase.Ready;
    return active && SampleLoader.sameSource(state.request, request);
  }

  /** Same request apart from a format override (an override must survive effect re-runs). */
  private static sameSource(a: LoadSamplesRequest | null, b: LoadSamplesRequest): boolean {
    if (!a) return false;
    return JSON.stringify({ ...a, format: undefined }) === JSON.stringify({ ...b, format: undefined });
  }

  private discard(sessionId: string): false {
    void this.service.close(sessionId).catch(() => undefined);
    return false;
  }

  private async rest(sessionId: string, first: SampleRecord[], cursor: string | undefined): Promise<SampleRecord[]> {
    const records = [...first];
    let next = cursor;
    while (next != null && records.length < PreviewLimit.MaxEvents) {
      const page = await this.service.listEvents(sessionId, { cursor: next || undefined, limit: PreviewLimit.PageSize });
      records.push(...page.events);
      next = page.nextCursor;
    }
    return records;
  }

  private async loadSchema(sessionId: string, generation: number): Promise<void> {
    try {
      const schema = await this.service.schema(sessionId);
      if (generation === this.generation) this.store.setState({ schema });
    } catch {
      if (generation === this.generation) this.store.setState({ schema: null });
    }
  }
}
