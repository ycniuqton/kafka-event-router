import { expect } from '@playwright/test';
import type { ApiResponse } from './Api';
import type { Workspace } from './Platform';
import { RouteDraft, type Json } from './RouteDraft';

/** One preview run of a draft over uploaded events, with accessors for the per-event trace. */
export class PreviewResult {
  constructor(readonly res: ApiResponse) {}

  get summary(): Json {
    return this.res.body.summary;
  }

  get results(): any[] {
    return this.res.body.results ?? [];
  }

  /** Output values of every passed event, in order (split events flattened). */
  get values(): any[] {
    return this.outputs.map((o) => o.value);
  }

  get outputs(): any[] {
    return this.results.flatMap((r) => r.branches.flatMap((b: any) => b.outputs ?? []));
  }

  statuses(): string[] {
    return this.results.map((r) => r.status);
  }

  /** Errors of every step trace and branch, flattened. */
  errors(): any[] {
    return this.results.flatMap((r) => [r.error, ...r.branches.flatMap((b: any) => [b.error, ...b.steps.map((s: any) => s.error)])]).filter(Boolean);
  }

  logs(): string[] {
    return this.results.flatMap((r) => r.branches.flatMap((b: any) => b.steps.flatMap((s: any) => (s.logs ?? []).map((l: any) => (typeof l === 'string' ? l : l.message ?? JSON.stringify(l))))));
  }
}

/** Runs transform steps / filters through the preview service (same engine as the workers). */
export class StudioRunner {
  private connectionId: string | null = null;

  constructor(private readonly ws: Workspace) {}

  /** Events are `{ key?, value, headers? }`; plain objects are wrapped as values. */
  async run(steps: Json[], events: Json[], filter?: Json): Promise<PreviewResult> {
    const conn = await this.connection();
    const draft = new RouteDraft(this.ws.names.route, conn, [this.ws.names.inTopic]).branch({ topic: this.ws.names.outTopic, steps, filter }).build();
    const session = await this.ws.uploadSession(events.map(StudioRunner.wrap));
    const res = await this.ws.previewRun(session, draft);
    expect(res.status, res.describe()).toBe(200);
    return new PreviewResult(res);
  }

  /** Single event, returns the first output value (fails when the event did not pass). */
  async one(steps: Json[], value: Json): Promise<any> {
    const result = await this.run(steps, [value]);
    expect(result.statuses(), JSON.stringify(result.results)).toEqual(['passed']);
    return result.values[0];
  }

  private async connection(): Promise<string> {
    if (!this.connectionId) this.connectionId = await this.ws.connection({}, this.ws.names.of('preview-conn'));
    return this.connectionId;
  }

  private static wrap(e: Json): Json {
    return 'value' in e ? e : { value: e };
  }
}
