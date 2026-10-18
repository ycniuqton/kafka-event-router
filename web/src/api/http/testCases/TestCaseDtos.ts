import type { Step } from '../../generated';
import type { TestCase, TestCaseRunReport } from '../../../domain';

/** Endpoints of saved test cases (cmd/api `internal/library/testcaseshttp`). */
export class TestCaseEndpoint {
  static collection(routeId: string): string {
    return `/routes/${encodeURIComponent(routeId)}/test-cases`;
  }

  static item(routeId: string, caseId: string): string {
    return `${TestCaseEndpoint.collection(routeId)}/${encodeURIComponent(caseId)}`;
  }

  static run(routeId: string): string {
    return `${TestCaseEndpoint.collection(routeId)}/run`;
  }
}

/** Case and report bodies are the domain shapes (JSON members match one to one). */
export type TestCaseDto = TestCase;
export type TestCaseListDto = { items: TestCaseDto[] };
export type TestCaseRunDto = TestCaseRunReport;
export type TestCaseRunRequestDto = { steps: Step[]; caseIds?: string[] };
