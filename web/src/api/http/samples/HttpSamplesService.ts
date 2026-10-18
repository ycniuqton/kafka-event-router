import type { SampleEvent } from '../../../domain';
import { SamplesService, type SampleField } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpPreviewClient, PreviewErrors } from '../preview/HttpPreviewClient';
import { PreviewLimit, SampleMode, type LoadSamplesResponse } from '../preview/PreviewDtos';
import { SampleMapper } from './SampleMapper';

/** How long a loaded topic sample is reused before a fresh load (one session, one audit entry). */
export const SampleCachePolicy = { reuseMs: 60_000, keySeparator: '\u0000' } as const;

type CachedLoad = { at: number; load: Promise<LoadSamplesResponse> };

/**
 * `services.samples` over live preview: `list` loads the latest events into a sample session,
 * `peek` is the same call with `count=20` (connections' topic Peek), `fields` reads the session schema.
 */
export class HttpSamplesService extends SamplesService {
  private readonly preview: HttpPreviewClient;
  private readonly now: () => number;
  private readonly cache = new Map<string, CachedLoad>();
  private lastSessionId: string | null = null;

  constructor(context: HttpContext, now: () => number = () => Date.now()) {
    super();
    this.preview = new HttpPreviewClient(context);
    this.now = now;
  }

  async list(connectionId: string, topic: string): Promise<SampleEvent[]> {
    if (!connectionId || !topic) return [];
    const res = await this.load(connectionId, topic, PreviewLimit.DefaultCount);
    return SampleMapper.events(res.events);
  }

  async peek(connectionId: string, topic: string, count: number): Promise<SampleEvent[]> {
    const res = await this.load(connectionId, topic, count);
    return SampleMapper.events(res.events).reverse();
  }

  async fields(): Promise<SampleField[]> {
    if (!this.lastSessionId) return [];
    try {
      return SampleMapper.fields(await this.preview.schema(this.lastSessionId));
    } catch (error) {
      if (PreviewErrors.isSessionExpired(error)) return [];
      throw error;
    }
  }

  private load(connectionId: string, topic: string, count: number): Promise<LoadSamplesResponse> {
    const key = [connectionId, topic, count].join(SampleCachePolicy.keySeparator);
    const cached = this.cache.get(key);
    if (cached && this.now() - cached.at < SampleCachePolicy.reuseMs) return cached.load;
    const load = this.preview.loadSamples({ connectionId, topics: [topic], mode: SampleMode.Latest, count });
    this.cache.set(key, { at: this.now(), load });
    load.then(
      (res) => (this.lastSessionId = res.session.id),
      () => this.cache.delete(key),
    );
    return load;
  }
}
