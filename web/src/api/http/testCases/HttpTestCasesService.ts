import type { NewTestCase, TestCase, TestCasePatch, TestCaseRunReport } from '../../../domain';
import { StepMapper } from '../../../domain/routeConfig/StepMapper';
import { TestCasesService, type TestCaseDraft } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { EntityTag } from '../members/MemberDtos';
import { TestCaseEndpoint, type TestCaseDto, type TestCaseListDto, type TestCaseRunDto, type TestCaseRunRequestDto } from './TestCaseDtos';

/** `services.testCases` over `/routes/{id}/test-cases`; updates send the case version as If-Match. */
export class HttpTestCasesService extends TestCasesService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(routeId: string): Promise<TestCase[]> {
    const res = await this.context.api.get<TestCaseListDto>(TestCaseEndpoint.collection(routeId));
    return res.items;
  }

  create(routeId: string, testCase: NewTestCase): Promise<TestCase> {
    return this.context.api.post<TestCaseDto>(TestCaseEndpoint.collection(routeId), testCase);
  }

  update(testCase: TestCase, patch: TestCasePatch): Promise<TestCase> {
    return this.context.api.patch<TestCaseDto>(TestCaseEndpoint.item(testCase.routeId, testCase.id), patch, { ifMatch: EntityTag.of(testCase.version) });
  }

  remove(testCase: TestCase): Promise<void> {
    return this.context.api.delete(TestCaseEndpoint.item(testCase.routeId, testCase.id));
  }

  run(routeId: string, draft: TestCaseDraft): Promise<TestCaseRunReport> {
    const body: TestCaseRunRequestDto = { steps: draft.steps.map((s) => StepMapper.toStep(s)), caseIds: draft.caseIds };
    return this.context.api.post<TestCaseRunDto>(TestCaseEndpoint.run(routeId), body, { silent: true });
  }
}
