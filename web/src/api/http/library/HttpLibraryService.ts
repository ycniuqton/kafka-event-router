import {
  LibraryImportError,
  LibraryKind,
  type LibraryLookupTable,
  type LibraryQuery,
  type LibraryTransform,
  type LibraryUsage,
  type LibraryVersion,
  type LookupRowsInput,
  type LookupRowsPage,
  type NewLibraryTransform,
  type NewLookupTable,
  type NewTestCase,
  type TestCase,
  type TestCasePatch,
  type TestCaseRunReport,
  type TransformStep,
} from '../../../domain';
import { StepMapper } from '../../../domain/routeConfig/StepMapper';
import { LibraryService, type LibrarySaveOptions } from '../../contracts';
import { ApiError, RawBody } from '../core';
import type { HttpContext } from '../core/HttpContext';
import { EntityTag } from '../members/MemberDtos';
import type { TestCaseDto, TestCaseListDto, TestCaseRunDto } from '../testCases/TestCaseDtos';
import {
  LibraryEndpoint,
  LibraryImport,
  LibraryMapper,
  LibraryQueryKey,
  type LibraryItemDto,
  type LibraryListDto,
  type LookupWriteDto,
  type RowsDto,
  type TransformWriteDto,
  type UsageDto,
  type VersionDto,
} from './LibraryDtos';

/** `services.library` over `/api/v1/transforms` and `/api/v1/lookup-tables` (no mock fallback). */
export class HttpLibraryService extends LibraryService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(query: LibraryQuery = {}): Promise<LibraryTransform[]> {
    return (await this.items(LibraryKind.Transform, query)).map((dto) => LibraryMapper.transform(dto));
  }

  async lookupTables(query: LibraryQuery = {}): Promise<LibraryLookupTable[]> {
    return (await this.items(LibraryKind.LookupTable, query)).map((dto) => LibraryMapper.lookupTable(dto));
  }

  async getTransform(id: string): Promise<LibraryTransform> {
    return LibraryMapper.transform(await this.context.api.get<LibraryItemDto>(LibraryEndpoint.item(LibraryKind.Transform, id)));
  }

  async getLookupTable(id: string): Promise<LibraryLookupTable> {
    return LibraryMapper.lookupTable(await this.context.api.get<LibraryItemDto>(LibraryEndpoint.item(LibraryKind.LookupTable, id)));
  }

  /** Publishes a new version when a transform of that name exists, else creates it. */
  async saveTransform(name: string, steps: TransformStep[], options: LibrarySaveOptions = {}): Promise<LibraryTransform> {
    const existing = (await this.items(LibraryKind.Transform, { names: [name] })).find((t) => t.name === name);
    if (existing) return this.publishVersion(existing.id, steps, options.changeNote ?? '');
    return this.createTransform({ name, steps, description: options.description ?? '', changeNote: options.changeNote ?? '' });
  }

  async createTransform(t: NewLibraryTransform): Promise<LibraryTransform> {
    const body: TransformWriteDto = { name: t.name, description: t.description, changeNote: t.changeNote, steps: t.steps.map((s) => StepMapper.toStep(s)) };
    return LibraryMapper.transform(await this.context.api.post<LibraryItemDto>(LibraryEndpoint.transforms, body));
  }

  async publishVersion(id: string, steps: TransformStep[], changeNote: string): Promise<LibraryTransform> {
    const body: TransformWriteDto = { steps: steps.map((s) => StepMapper.toStep(s)), changeNote };
    return LibraryMapper.transform(await this.context.api.post<LibraryItemDto>(LibraryEndpoint.versions(LibraryKind.Transform, id), body));
  }

  async createLookupTable(t: NewLookupTable): Promise<LibraryLookupTable> {
    const body: LookupWriteDto = { name: t.name, description: t.description, keyType: t.keyType, rows: t.rows, changeNote: t.changeNote };
    return LibraryMapper.lookupTable(await this.context.api.post<LibraryItemDto>(LibraryEndpoint.lookupTables, body));
  }

  /** Grid rows go as JSON; CSV text is uploaded as `text/csv` and imported line by line by the server. */
  async publishLookupRows(id: string, input: LookupRowsInput, changeNote: string): Promise<LibraryLookupTable> {
    const path = LibraryEndpoint.versions(LibraryKind.LookupTable, id);
    try {
      const dto = 'csv' in input
        ? await this.context.api.post<LibraryItemDto>(path, new RawBody(input.csv, LibraryImport.csv), { query: { [LibraryQueryKey.ChangeNote]: changeNote } })
        : await this.context.api.post<LibraryItemDto>(path, { rows: input.rows, changeNote } satisfies LookupWriteDto);
      return LibraryMapper.lookupTable(dto);
    } catch (e) {
      const issues = e instanceof ApiError ? LibraryMapper.importIssues(e.errors) : [];
      throw issues.length ? new LibraryImportError(issues) : e;
    }
  }

  async versions(kind: LibraryKind, id: string): Promise<LibraryVersion[]> {
    const res = await this.context.api.get<{ items: VersionDto[] }>(LibraryEndpoint.versions(kind, id));
    return res.items.map(({ steps: _steps, rows: _rows, ...v }) => LibraryMapper.version(v));
  }

  async version(kind: LibraryKind, id: string, version: number): Promise<LibraryVersion> {
    return LibraryMapper.version(await this.context.api.get<VersionDto>(LibraryEndpoint.version(kind, id, version)));
  }

  async usages(kind: LibraryKind, id: string): Promise<LibraryUsage[]> {
    return (await this.context.api.get<{ items: UsageDto[] }>(LibraryEndpoint.usages(kind, id))).items;
  }

  async rows(id: string, version: number, cursor?: string | null): Promise<LookupRowsPage> {
    const query = cursor ? { [LibraryQueryKey.Cursor]: cursor } : undefined;
    return LibraryMapper.rows(await this.context.api.get<RowsDto>(LibraryEndpoint.rows(id, version), query));
  }

  async updateDescription<T extends LibraryTransform>(kind: LibraryKind, item: T, description: string): Promise<T> {
    const dto = await this.context.api.patch<LibraryItemDto>(LibraryEndpoint.item(kind, item.id), { description }, { ifMatch: EntityTag.of(item.rowVersion) });
    return (kind === LibraryKind.LookupTable ? LibraryMapper.lookupTable(dto) : LibraryMapper.transform(dto)) as T;
  }

  archive(kind: LibraryKind, id: string): Promise<void> {
    return this.context.api.delete(LibraryEndpoint.item(kind, id));
  }

  async latestVersions(kind: LibraryKind, names: string[]): Promise<Record<string, number>> {
    if (!names.length) return {};
    const items = await this.items(kind, { names });
    return Object.fromEntries(items.map((i) => [i.name, i.latestVersion]));
  }

  async testCases(transformId: string): Promise<TestCase[]> {
    return (await this.context.api.get<TestCaseListDto>(LibraryEndpoint.testCases(transformId))).items;
  }

  addTestCase(transformId: string, testCase: NewTestCase): Promise<TestCase> {
    return this.context.api.post<TestCaseDto>(LibraryEndpoint.testCases(transformId), testCase);
  }

  updateTestCase(testCase: TestCase, patch: TestCasePatch): Promise<TestCase> {
    return this.context.api.patch<TestCaseDto>(LibraryEndpoint.testCase(testCase.routeId, testCase.id), patch, { ifMatch: EntityTag.of(testCase.version) });
  }

  removeTestCase(testCase: TestCase): Promise<void> {
    return this.context.api.delete(LibraryEndpoint.testCase(testCase.routeId, testCase.id));
  }

  runTestCases(transformId: string, steps?: TransformStep[], caseIds?: string[]): Promise<TestCaseRunReport> {
    const body = { steps: steps?.map((s) => StepMapper.toStep(s)), caseIds };
    return this.context.api.post<TestCaseRunDto>(LibraryEndpoint.testRun(transformId), body);
  }

  private async items(kind: LibraryKind, query: LibraryQuery): Promise<LibraryItemDto[]> {
    const params = {
      [LibraryQueryKey.Text]: query.text || undefined,
      [LibraryQueryKey.Names]: query.names?.join(LibraryImport.namesSeparator),
      [LibraryQueryKey.Limit]: LibraryImport.listLimit,
    };
    return (await this.context.api.get<LibraryListDto>(LibraryEndpoint.collections[kind], params)).items;
  }
}
