import {
  LookupHealthStatus,
  LookupKindSettingsOf,
  LookupSourceInUseError,
  LookupSourceKindMeta,
  LookupSourceRules,
  LookupTestState,
  LookupTestStep,
  type LookupKindSettings,
  type LookupSource,
  type LookupSourceDraft,
  type LookupTestResult,
  type LookupTestStepResult,
} from '../../domain';
import { TEAMS } from '../../mocks';
import { LookupSourcesService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Texts and markers of the scripted mock test. */
const MockLookupTest = {
  unreachableMarker: 'unreachable',
  unreachable: 'The source did not answer within 10 s.',
  hint: 'Check the address and that the router can reach it.',
  inUse: 'Routes still use this lookup source.',
  stepMs: 12,
  idPrefix: 'ls-',
} as const;

/** In-memory lookup sources with a scripted test (a target containing "unreachable" fails the first step). */
export class MockLookupSourcesService extends LookupSourcesService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(): Promise<LookupSource[]> {
    return this.backend.reply(this.backend.db.lookupSources);
  }

  get(id: string): Promise<LookupSource> {
    return this.backend.reply(this.require(id));
  }

  create(draft: LookupSourceDraft): Promise<LookupSource> {
    const now = this.backend.nowIso();
    const source: LookupSource = {
      ...this.fromDraft(draft, `${MockLookupTest.idPrefix}${draft.name || now}`),
      health: { status: LookupHealthStatus.Unknown, checkedAt: null, lastFailure: null },
      usedBy: { count: 0, routes: [] },
      version: 1,
      createdAt: now,
      updatedAt: now,
    };
    this.backend.db.lookupSources = [...this.backend.db.lookupSources, source];
    return this.backend.reply(source);
  }

  update(id: string, draft: LookupSourceDraft): Promise<LookupSource> {
    const current = this.require(id);
    const fields = new Set([...current.secretFields, ...this.fromDraft(draft, id).secretFields]);
    const updated: LookupSource = { ...current, ...this.fromDraft(draft, id), secretFields: [...fields], version: current.version + 1, updatedAt: this.backend.nowIso() };
    this.backend.db.lookupSources = this.backend.db.lookupSources.map((s) => (s.id === id ? updated : s));
    return this.backend.reply(updated);
  }

  async remove(id: string): Promise<void> {
    const current = this.require(id);
    if (current.usedBy.count > 0) throw new LookupSourceInUseError(MockLookupTest.inUse, current.usedBy.routes);
    this.backend.db.lookupSources = this.backend.db.lookupSources.filter((s) => s.id !== id);
    await this.backend.reply(null);
  }

  async test(id: string): Promise<LookupTestResult> {
    const current = this.require(id);
    const result = this.run(current);
    const health = { status: result.health, checkedAt: result.checkedAt, lastFailure: null };
    this.backend.db.lookupSources = this.backend.db.lookupSources.map((s) => (s.id === id ? { ...s, health } : s));
    return this.backend.reply(result);
  }

  testDraft(draft: LookupSourceDraft): Promise<LookupTestResult> {
    return this.backend.reply(this.run(this.fromDraft(draft, draft.name)));
  }

  private run(settings: LookupKindSettings & Pick<LookupSource, 'kind'>): LookupTestResult {
    const target = LookupSourceKindMeta[settings.kind].target(settings);
    const ok = !target.includes(MockLookupTest.unreachableMarker);
    const steps: LookupTestStepResult[] = ok
      ? Object.values(LookupTestStep).map((step) => ({ step, state: LookupTestState.Passed, durationMs: MockLookupTest.stepMs }))
      : [
          { step: LookupTestStep.Reachable, state: LookupTestState.Failed, durationMs: MockLookupTest.stepMs, message: MockLookupTest.unreachable, hint: MockLookupTest.hint },
          { step: LookupTestStep.Authenticated, state: LookupTestState.Skipped },
          { step: LookupTestStep.Sample, state: LookupTestState.Skipped },
        ];
    return { ok, health: ok ? LookupHealthStatus.Healthy : LookupHealthStatus.Unreachable, checkedAt: this.backend.nowIso(), steps };
  }

  private fromDraft(d: LookupSourceDraft, id: string): Omit<LookupSource, 'health' | 'usedBy' | 'version' | 'createdAt' | 'updatedAt'> {
    const team = TEAMS.find((t) => t.id === d.ownerTeamId) ?? TEAMS[0];
    const secretFields = LookupSourceRules.secretFields(d).filter((f) => !!d.secrets[f]?.trim());
    return {
      id, name: d.name, description: d.description, kind: d.kind, envTag: d.envTag, sensitive: d.sensitive,
      owner: { teamId: team.id, teamName: team.name },
      ...LookupKindSettingsOf.draft(d),
      limits: d.limits, secretFields,
    };
  }

  private require(id: string): LookupSource {
    const found = this.backend.db.lookupSources.find((s) => s.id === id);
    if (!found) throw new Error(`Unknown lookup source ${id}`);
    return found;
  }
}
