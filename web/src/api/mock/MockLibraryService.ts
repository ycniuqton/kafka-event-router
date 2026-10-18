import {
  LibraryImportError,
  LibraryKind,
  LibraryRules,
  LookupCsv,
  type LibraryLookupTable,
  type LibraryQuery,
  type LibraryTransform,
  type LibraryUsage,
  type LibraryVersion,
  type LookupRow,
  type LookupRowsInput,
  type LookupRowsPage,
  type NewLibraryTransform,
  type NewLookupTable,
  type NewTestCase,
  type TestCase,
  type TestCasePatch,
  type TestCaseRunReport,
  type TransformStep,
} from '../../domain';
import { StepMapper } from '../../domain/routeConfig/StepMapper';
import { CURRENT_USER, LIBRARY, LIBRARY_VERSIONS, LOOKUP_ROWS, LOOKUP_TABLES } from '../../mocks';
import { LibraryService, type LibrarySaveOptions } from '../contracts';
import type { MockBackend } from './MockBackend';
import { MockTestCasesService } from './MockTestCasesService';

/** Texts of the mock server's library errors (same wording as the API). */
export const MockLibraryText = {
  nameTaken: 'A library object with this name already exists. Publish a new version of it or pick another name.',
  notFound: 'The requested resource does not exist. It may have been deleted.',
  inUse: (n: number) => `Used by ${n} route(s). Remove the references before archiving.`,
  invalidName: 'Use lower-case letters, digits and dashes (e.g. mask-customer-pii).',
} as const;

/** Paging and identity of the mock library. */
const MockLibraryPaging = { rowsPage: 50, idPrefix: 'lib-new-', tablePrefix: 'lt-new-', hashPrefix: 'sha256:mock-' } as const;

type MockVersion = LibraryVersion & { steps?: TransformStep[]; rows?: LookupRow[] };

/** In-memory library: transforms and lookup tables with immutable versions, usages and transform test cases. */
export class MockLibraryService extends LibraryService {
  private readonly backend: MockBackend;
  private readonly transforms: LibraryTransform[] = structuredClone(LIBRARY);
  private readonly tables: LibraryLookupTable[] = structuredClone(LOOKUP_TABLES);
  private readonly history = new Map<string, MockVersion[]>();
  private readonly cases: MockTestCasesService;
  private lastId = 0;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
    this.cases = new MockTestCasesService(backend);
    this.seedHistory();
  }

  list(query: LibraryQuery = {}): Promise<LibraryTransform[]> {
    return this.backend.reply(MockLibraryFilter.apply(this.transforms, query));
  }

  lookupTables(query: LibraryQuery = {}): Promise<LibraryLookupTable[]> {
    return this.backend.reply(MockLibraryFilter.apply(this.tables, query));
  }

  getTransform(id: string): Promise<LibraryTransform> {
    const found = this.transforms.find((t) => t.id === id);
    return found ? this.backend.reply(found) : this.backend.fail(MockLibraryText.notFound);
  }

  getLookupTable(id: string): Promise<LibraryLookupTable> {
    const found = this.tables.find((t) => t.id === id);
    return found ? this.backend.reply(found) : this.backend.fail(MockLibraryText.notFound);
  }

  saveTransform(name: string, steps: TransformStep[], options: LibrarySaveOptions = {}): Promise<LibraryTransform> {
    const existing = this.transforms.find((t) => t.name === name);
    if (existing) return this.publishVersion(existing.id, steps, options.changeNote ?? '');
    return this.createTransform({ name, steps, description: options.description ?? '', changeNote: options.changeNote ?? '' });
  }

  createTransform(input: NewLibraryTransform): Promise<LibraryTransform> {
    const refused = this.refuseName(input.name, this.transforms);
    if (refused) return this.backend.fail(refused);
    const created = this.newItem(input.name, input.description, input.steps.length, MockLibraryPaging.idPrefix);
    this.transforms.push(created);
    this.history.set(created.id, [this.versionOf(1, input.changeNote, { steps: input.steps })]);
    return this.backend.reply(created);
  }

  publishVersion(id: string, steps: TransformStep[], changeNote: string): Promise<LibraryTransform> {
    const item = this.transforms.find((t) => t.id === id);
    if (!item) return this.backend.fail(MockLibraryText.notFound);
    this.bump(item, steps.length, this.versionOf(item.latestVersion + 1, changeNote, { steps }));
    return this.backend.reply(item);
  }

  createLookupTable(input: NewLookupTable): Promise<LibraryLookupTable> {
    const refused = this.refuseName(input.name, this.tables);
    if (refused) return this.backend.fail(refused);
    const created: LibraryLookupTable = { ...this.newItem(input.name, input.description, input.rows.length, MockLibraryPaging.tablePrefix), rows: input.rows.length, keyType: input.keyType };
    this.tables.push(created);
    this.history.set(created.id, [this.versionOf(1, input.changeNote, { rows: input.rows })]);
    return this.backend.reply(created);
  }

  publishLookupRows(id: string, input: LookupRowsInput, changeNote: string): Promise<LibraryLookupTable> {
    const item = this.tables.find((t) => t.id === id);
    if (!item) return this.backend.fail(MockLibraryText.notFound);
    const parsed = 'csv' in input ? LookupCsv.parse(input.csv) : { rows: input.rows, issues: [] };
    if (parsed.issues.length) return Promise.reject(new LibraryImportError(parsed.issues));
    this.bump(item, parsed.rows.length, this.versionOf(item.latestVersion + 1, changeNote, { rows: parsed.rows }));
    item.rows = parsed.rows.length;
    return this.backend.reply(item);
  }

  versions(_kind: LibraryKind, id: string): Promise<LibraryVersion[]> {
    return this.backend.reply([...(this.history.get(id) ?? [])].reverse().map(({ steps: _s, rows: _r, ...v }) => v));
  }

  version(_kind: LibraryKind, id: string, version: number): Promise<LibraryVersion> {
    const found = this.history.get(id)?.find((v) => v.version === version);
    return found ? this.backend.reply(found) : this.backend.fail(MockLibraryText.notFound);
  }

  /** Seeded objects are "used" by the first `usedBy` routes of the mock database, pinned to their latest version. */
  usages(kind: LibraryKind, id: string): Promise<LibraryUsage[]> {
    const item = this.itemOf(kind, id);
    const routes = item ? this.backend.db.routes.slice(0, item.usedBy) : [];
    return this.backend.reply(routes.map((r) => ({ routeId: r.id, routeName: r.name, versions: [item?.latestVersion ?? 1], inDraft: false, inActive: true })));
  }

  rows(id: string, version: number, cursor?: string | null): Promise<LookupRowsPage> {
    const all = this.history.get(id)?.find((v) => v.version === version)?.rows ?? [];
    const from = Number(cursor ?? 0);
    const to = from + MockLibraryPaging.rowsPage;
    return this.backend.reply({ rows: all.slice(from, to), total: all.length, next: to < all.length ? String(to) : null });
  }

  updateDescription<T extends LibraryTransform>(kind: LibraryKind, item: T, description: string): Promise<T> {
    const current = this.itemOf(kind, item.id);
    if (!current) return this.backend.fail(MockLibraryText.notFound);
    current.description = description;
    current.rowVersion += 1;
    return this.backend.reply(current as T);
  }

  async archive(kind: LibraryKind, id: string): Promise<void> {
    const usages = await this.usages(kind, id);
    const current = this.itemOf(kind, id);
    if (!current) return this.backend.fail(MockLibraryText.notFound);
    if (usages.length) return this.backend.fail(MockLibraryText.inUse(usages.length));
    current.archived = true;
    return this.backend.reply(undefined);
  }

  latestVersions(kind: LibraryKind, names: string[]): Promise<Record<string, number>> {
    const items: LibraryTransform[] = kind === LibraryKind.Transform ? this.transforms : this.tables;
    return this.backend.reply(Object.fromEntries(items.filter((i) => names.includes(i.name)).map((i) => [i.name, i.latestVersion])));
  }

  testCases(transformId: string): Promise<TestCase[]> {
    return this.cases.list(transformId);
  }

  addTestCase(transformId: string, testCase: NewTestCase): Promise<TestCase> {
    return this.cases.create(transformId, testCase);
  }

  updateTestCase(testCase: TestCase, patch: TestCasePatch): Promise<TestCase> {
    return this.cases.update(testCase, patch);
  }

  removeTestCase(testCase: TestCase): Promise<void> {
    return this.cases.remove(testCase);
  }

  runTestCases(transformId: string, steps?: TransformStep[], caseIds?: string[]): Promise<TestCaseRunReport> {
    const latest = this.history.get(transformId)?.at(-1)?.steps ?? [];
    return this.cases.run(transformId, { steps: steps ?? latest, caseIds });
  }

  private itemOf(kind: LibraryKind, id: string): LibraryTransform | undefined {
    const items: LibraryTransform[] = kind === LibraryKind.Transform ? this.transforms : this.tables;
    return items.find((i) => i.id === id);
  }

  private refuseName(name: string, items: LibraryTransform[]): string | null {
    if (!LibraryRules.validName(name)) return MockLibraryText.invalidName;
    return items.some((i) => i.name === name) ? MockLibraryText.nameTaken : null;
  }

  private newItem(name: string, description: string, size: number, prefix: string): LibraryTransform {
    this.lastId += 1;
    return {
      id: `${prefix}${this.lastId}`, name, description, version: LibraryRules.versionLabel(1), latestVersion: 1, usedBy: 0,
      team: CURRENT_USER.team, teamId: CURRENT_USER.team, size, rowVersion: 1, archived: false, updatedAt: this.backend.nowIso(),
    };
  }

  private bump(item: LibraryTransform, size: number, version: MockVersion): void {
    item.latestVersion = version.version;
    item.version = LibraryRules.versionLabel(version.version);
    item.size = size;
    item.updatedAt = version.createdAt;
    this.history.set(item.id, [...(this.history.get(item.id) ?? []), version]);
  }

  private versionOf(version: number, changeNote: string, content: { steps?: TransformStep[]; rows?: LookupRow[] }): MockVersion {
    const size = content.steps?.length ?? content.rows?.length ?? 0;
    return { version, size, changeNote, configHash: `${MockLibraryPaging.hashPrefix}${version}`, createdAt: this.backend.nowIso(), createdBy: CURRENT_USER.name, ...content };
  }

  private seedHistory(): void {
    for (const t of this.transforms) {
      const versions = (LIBRARY_VERSIONS[t.id] ?? []).map((steps, i) => this.versionOf(i + 1, '', { steps: steps.flatMap((s) => StepMapper.fromStep(s) ?? []) }));
      this.history.set(t.id, versions);
    }
    for (const t of this.tables) this.history.set(t.id, [this.versionOf(t.latestVersion, '', { rows: LOOKUP_ROWS[t.id] ?? [] })]);
  }
}

/** Name filter of mock listings (`q` fragment, exact `names`), archived objects hidden. */
class MockLibraryFilter {
  static apply<T extends LibraryTransform>(items: T[], query: LibraryQuery): T[] {
    const text = query.text?.trim().toLowerCase() ?? '';
    return items.filter((i) => !i.archived && (!text || i.name.includes(text)) && (!query.names || query.names.includes(i.name)));
  }
}
