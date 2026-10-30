import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

/** Behaviour of the fake CRM the lookup-source cases call (served by the test runner on 127.0.0.1). */
export const FakeCrmSpec = {
  host: '127.0.0.1',
  token: 'acc-crm-token',
  customersPath: '/customers/',
  probePath: '/',
  customers: { c1: { tier: 'gold', region: 'eu' }, c2: { tier: 'silver', region: 'us' } } as Record<string, { tier: string; region: string }>,
  status: { ok: 200, unauthorized: 401, notFound: 404, unavailable: 503 },
} as const;

/** A tiny HTTP CRM: bearer-protected `/customers/<id>`, switchable outage, request log. */
export class FakeCrm {
  readonly requests: string[] = [];
  private server: Server | null = null;
  private failuresLeft = 0;
  private down = false;

  get baseUrl(): string {
    const addr = this.server?.address() as AddressInfo | undefined;
    return `http://${FakeCrmSpec.host}:${addr?.port ?? 0}`;
  }

  async start(): Promise<this> {
    this.server = createServer((req, res) => this.handle(req, res));
    await new Promise<void>((resolve) => this.server!.listen(0, FakeCrmSpec.host, resolve));
    return this;
  }

  async stop(): Promise<void> {
    await new Promise<void>((resolve) => (this.server ? this.server.close(() => resolve()) : resolve()));
  }

  /** Every call answers 503 until `up()`. */
  outage(): void {
    this.down = true;
  }

  up(): void {
    this.down = false;
  }

  /** The next `n` customer calls answer 503, then the CRM recovers. */
  failNext(n: number): void {
    this.failuresLeft = n;
  }

  private handle(req: IncomingMessage, res: ServerResponse): void {
    const url = req.url ?? FakeCrmSpec.probePath;
    this.requests.push(url);
    if (req.headers.authorization !== `Bearer ${FakeCrmSpec.token}`) return this.reply(res, FakeCrmSpec.status.unauthorized, { error: 'unauthorized' });
    if (!url.startsWith(FakeCrmSpec.customersPath)) return this.reply(res, FakeCrmSpec.status.ok, { ok: true });
    if (this.down || this.failuresLeft > 0) {
      this.failuresLeft = Math.max(0, this.failuresLeft - 1);
      return this.reply(res, FakeCrmSpec.status.unavailable, { error: 'down' });
    }
    const row = FakeCrmSpec.customers[decodeURIComponent(url.slice(FakeCrmSpec.customersPath.length)).split('?')[0]];
    this.reply(res, row ? FakeCrmSpec.status.ok : FakeCrmSpec.status.notFound, row ?? { error: 'not found' });
  }

  private reply(res: ServerResponse, status: number, body: unknown): void {
    res.writeHead(status, { 'content-type': 'application/json' });
    res.end(JSON.stringify(body));
  }
}

/** Limits that make breaker / retries deterministic in the acceptance cases. */
export const AccLookupLimits = { maxConcurrency: 8, ratePerSec: 0, burst: 0, breakerFailures: 0, breakerOpenMs: 0 } as const;

/** Builds lookup-source bodies and lookup steps for the suite. */
export class LookupSources {
  static readonly path = '/api/v1/lookup-sources';

  static item(id: string): string {
    return `${LookupSources.path}/${id}`;
  }

  /** HTTP source (bearer) pointed at `baseUrl`. */
  static http(name: string, teamId: string, baseUrl: string, extra: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      name, kind: 'http', envTag: 'dev', owner: { teamId }, description: 'acceptance',
      http: { baseUrl, auth: { mode: 'bearer' }, tls: {} }, limits: AccLookupLimits, secrets: { token: FakeCrmSpec.token }, ...extra,
    };
  }

  /** A source-mode lookup step enriching value.customer from `/customers/${value.customerId}`. */
  static step(sourceId: string, extra: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      id: 'crm', kind: 'lookup',
      lookup: {
        sourceId, request: { kind: 'http', method: 'GET', path: '/customers/${value.customerId}' },
        map: [{ from: '$.tier', to: 'value.customer.tier' }, { from: '$.region', to: 'value.customer.region' }],
        timeoutMs: 1000, retries: 0, ...extra,
      },
    };
  }
}
