/** router/v1 vocabulary used by the drafts the suite builds. */
export const RouteDoc = {
  apiVersion: 'router/v1',
  mainBranch: 'main',
  start: { latest: 'latest', earliest: 'earliest' },
  onError: { dlq: 'dlq', skip: 'skip', pause: 'pause' },
  delivery: { atLeastOnce: 'at_least_once', exactlyOnce: 'exactly_once' },
  format: { json: 'json' },
} as const;

/** Step / filter node shapes are free-form JSON (owned by pipeline-engine's schema). */
export type Json = Record<string, unknown>;

/** One branch: filter + steps + destination topic (destination connection defaults to the source one). */
export type BranchSpec = { id?: string; filter?: Json; steps?: Json[]; topic: string; connectionId?: string; destination?: Json };

/** Fluent builder of a route document. */
export class RouteDraft {
  private readonly doc: Json;
  private readonly branches: Json[] = [];

  constructor(
    readonly name: string,
    private readonly connectionId: string,
    topics: string[],
  ) {
    this.doc = {
      apiVersion: RouteDoc.apiVersion,
      name,
      source: { connectionId, topics, format: { type: RouteDoc.format.json }, startFrom: { mode: RouteDoc.start.earliest }, consumerGroup: `router.${name}` },
      errorPolicy: { onError: RouteDoc.onError.dlq },
    };
  }

  branch(spec: BranchSpec): this {
    this.branches.push({
      id: spec.id ?? (this.branches.length ? `branch-${this.branches.length + 1}` : RouteDoc.mainBranch),
      ...(spec.filter ? { filter: spec.filter } : {}),
      transform: { steps: spec.steps ?? [] },
      destination: { connectionId: spec.connectionId ?? this.connectionId, topic: spec.topic, key: { mode: 'keep' }, partitioning: 'by_key', headers: { mode: 'keep_all' }, format: { type: RouteDoc.format.json }, ...(spec.destination ?? {}) },
    });
    return this;
  }

  startFrom(mode: string): this {
    (this.doc.source as Json).startFrom = { mode };
    return this;
  }

  onError(onError: string, extra: Json = {}): this {
    this.doc.errorPolicy = { onError, ...extra };
    return this;
  }

  delivery(mode: string): this {
    this.doc.delivery = { mode };
    return this;
  }

  limits(limits: Json): this {
    this.doc.limits = limits;
    return this;
  }

  set(key: string, value: unknown): this {
    this.doc[key] = value;
    return this;
  }

  build(): Json {
    return { ...this.doc, branches: this.branches };
  }
}

/** Filter AST helpers (pipeline-engine §7.3). */
export class Filter {
  static cond(field: string, op: string, value?: unknown): Json {
    return value === undefined ? { type: 'cond', field, op } : { type: 'cond', field, op, value };
  }

  static and(...children: Json[]): Json {
    return { type: 'and', children };
  }

  static or(...children: Json[]): Json {
    return { type: 'or', children };
  }

  static not(child: Json): Json {
    return { type: 'not', child };
  }

  static expr(code: string): Json {
    return { type: 'expr', code };
  }
}

/** Ready-made JS step bodies. */
export class JsSteps {
  static readonly identity = 'function transform(event, ctx) { return event; }';
  static readonly throwOnBad = 'function transform(event, ctx) { if (event.value.bad) { throw new Error("bad event " + event.value.id); } return event; }';
  static readonly infiniteLoop = 'function transform(event, ctx) { while (true) {} }';

  static step(code: string, id = 'js'): Json {
    return { id, kind: 'js', code };
  }

  /** Sets value.version = n (used to tell versions apart on the destination). */
  static tagVersion(n: number): Json {
    return Steps.setConstant('value.v', n);
  }
}

/** Recipe step helpers (pipeline-engine step kinds). */
export class Steps {
  static setConstant(field: string, value: unknown): Json {
    return { kind: 'set', field, source: { from: 'constant', value } };
  }

  static rename(from: string, to: string): Json {
    return { kind: 'rename', from, to };
  }

  static mask(field: string, style: 'full' | 'last4' | 'email'): Json {
    return { kind: 'mask', field, style };
  }

  static remove(...fields: string[]): Json {
    return { kind: 'remove', fields };
  }
}
