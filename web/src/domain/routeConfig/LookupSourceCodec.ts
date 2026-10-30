import type { LookupCache, LookupRequest, LookupSpec, LookupTemplates } from '../../api/generated';
import { LookupRequestKind } from '../lookupSource';
import {
  LookupErrorAction,
  LookupMissAction,
  LookupMockMode,
  LookupRedisOp,
  LookupStepDefaults,
  type LookupCacheDraft,
  type LookupKeyValue,
  type LookupRequestDraft,
  type LookupSourceStep,
} from '../lookupStep';
import { StepConstant } from '../transformOptions';

/** Value-relative Studio paths ↔ engine document paths (same convention as `EventPath`). */
class DocumentPath {
  static readonly valuePrefix = 'value.';

  static to(field: string): string {
    return DocumentPath.valuePrefix + field;
  }

  static from(path: string): string {
    return path.startsWith(DocumentPath.valuePrefix) ? path.slice(DocumentPath.valuePrefix.length) : path;
  }
}

/** Name → template rows ↔ engine template maps (blank names dropped). */
class TemplateRows {
  static toMap(rows: LookupKeyValue[]): LookupTemplates | undefined {
    const kept = rows.filter((r) => r.key.trim());
    return kept.length ? Object.fromEntries(kept.map((r) => [r.key.trim(), r.value])) : undefined;
  }

  static fromMap(map: LookupTemplates | undefined): LookupKeyValue[] {
    return Object.entries(map ?? {}).map(([key, value]) => ({ key, value }));
  }
}

/** Members each request kind sends (registry; other members are left out of the document). */
class RequestMembers {
  private static readonly writers: Record<LookupRequestKind, (r: LookupRequestDraft) => Partial<LookupRequest>> = {
    [LookupRequestKind.Http]: (r) => ({
      method: r.method,
      path: r.path,
      query: TemplateRows.toMap(r.query),
      headers: TemplateRows.toMap(r.headers),
      body: r.body.trim() ? StepConstant.parse(r.body) : undefined,
    }),
    [LookupRequestKind.Redis]: (r) => RequestMembers.redis(r),
    [LookupRequestKind.Sql]: (r) => ({ sql: r.sql, params: TemplateRows.toMap(r.params) }),
    [LookupRequestKind.Static]: (r) => ({ key: r.key }),
  };

  static of(r: LookupRequestDraft): LookupRequest {
    const members = RequestMembers.writers[r.kind](r);
    const defined = Object.entries(members).filter(([, v]) => v !== undefined);
    return { kind: r.kind, ...Object.fromEntries(defined) };
  }

  private static redis(r: LookupRequestDraft): Partial<LookupRequest> {
    if (r.op === LookupRedisOp.MGet) return { op: r.op, keys: r.keys.filter((k) => k.trim()) as LookupRequest['keys'] };
    if (r.op === LookupRedisOp.HGet) return { op: r.op, key: r.key, hashField: r.hashField };
    return { op: r.op, key: r.key };
  }
}

/** Lookup step source mode ↔ engine `lookup` member (pipeline-engine 00002 §1); the Studio mock stays out of the document. */
export class LookupSourceCodec {
  static toSpec(s: LookupSourceStep): LookupSpec {
    const spec: LookupSpec = {
      sourceId: s.sourceId,
      request: RequestMembers.of(s.request),
      map: s.map.filter((m) => m.to.trim()).map((m) => ({ from: m.from.trim(), to: DocumentPath.to(m.to.trim()) })) as LookupSpec['map'],
    };
    if (s.timeoutMs !== LookupStepDefaults.timeoutMs) spec.timeoutMs = s.timeoutMs;
    if (s.retries !== LookupStepDefaults.retries) spec.retries = s.retries;
    if (s.onMiss.action !== LookupMissAction.Skip) spec.onMiss = LookupSourceCodec.policy(s.onMiss.action, s.onMiss.value, LookupMissAction.Default);
    if (s.onError.action !== LookupErrorAction.Fail) spec.onError = LookupSourceCodec.policy(s.onError.action, s.onError.value, LookupErrorAction.Default);
    if (s.cache.enabled) spec.cache = LookupSourceCodec.cache(s.cache);
    return spec;
  }

  static fromSpec(spec: LookupSpec): LookupSourceStep {
    const d = LookupStepDefaults.step();
    return {
      ...d,
      sourceId: spec.sourceId,
      request: LookupSourceCodec.request(spec.request),
      map: spec.map.map((m) => ({ from: m.from, to: DocumentPath.from(m.to) })),
      timeoutMs: spec.timeoutMs ?? d.timeoutMs,
      retries: spec.retries ?? d.retries,
      onMiss: spec.onMiss ? { action: spec.onMiss.action, value: LookupSourceCodec.text(spec.onMiss.value) } : d.onMiss,
      onError: spec.onError ? { action: spec.onError.action, value: LookupSourceCodec.text(spec.onError.value) } : d.onError,
      cache: spec.cache ? LookupSourceCodec.cacheDraft(spec.cache) : d.cache,
      mockMode: LookupMockMode.Off,
    };
  }

  private static policy<A extends string>(action: A, value: string, withValue: string): { action: A; value?: unknown } {
    return action === withValue ? { action, value: StepConstant.parse(value) } : { action };
  }

  private static cache(c: LookupCacheDraft): LookupCache {
    const out: LookupCache = { ttlMs: c.ttlMs, maxEntries: c.maxEntries };
    if (c.negativeTtlMs > 0) out.negativeTtlMs = c.negativeTtlMs;
    return out;
  }

  private static cacheDraft(c: LookupCache): LookupCacheDraft {
    return { enabled: true, ttlMs: c.ttlMs ?? 0, negativeTtlMs: c.negativeTtlMs ?? 0, maxEntries: c.maxEntries ?? LookupStepDefaults.maxEntries };
  }

  private static request(r: LookupRequest): LookupRequestDraft {
    const d = LookupStepDefaults.request(r.kind as LookupRequestKind);
    return {
      ...d,
      method: r.method ?? d.method,
      path: r.path ?? '',
      query: TemplateRows.fromMap(r.query),
      headers: TemplateRows.fromMap(r.headers),
      body: r.body === undefined ? '' : JSON.stringify(r.body),
      op: r.op ?? d.op,
      key: r.key ?? '',
      keys: r.keys ?? [],
      hashField: r.hashField ?? '',
      sql: r.sql ?? '',
      params: TemplateRows.fromMap(r.params),
    };
  }

  private static text(value: unknown): string {
    return value === undefined ? '' : StepConstant.format(value);
  }
}
