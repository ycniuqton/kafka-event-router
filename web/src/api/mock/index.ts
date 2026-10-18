import type { Services } from '../contracts';
import { MockBackend } from './MockBackend';
import {
  MockActivityService,
  MockMembersService,
  MockSamplesService,
} from './MockCatalogServices';
import { MockAccessService } from './MockAccessServices';
import { MockMonitoring } from './MockMonitoringServices';
import { MockLiveTailService, MockOnboardingService } from './MockLiveTail';
import { MockApprovalsService } from './MockApprovalsService';
import { MockTeamsService } from './MockTeamsService';
import { MockApiTokensService } from './MockApiTokensService';
import { MockAuditService } from './MockAuditService';
import { MockAssistService } from './MockAssistService';
import { MockLibraryService } from './MockLibraryService';
import { MockConnectionsService } from './MockConnectionsService';
import { MockDeadLettersService } from './MockDeadLettersService';
import { MockRoutesService } from './MockRoutesService';
import { MockRuntimeService } from './MockRuntimeService';
import { MockSandboxService } from './MockSandboxService';
import { MockPreviewService } from './MockPreviewService';
import { MockTestCasesService } from './MockTestCasesService';

/** Builds the full mock service set over one shared in-memory backend. */
export class MockServicesFactory {
  static create(): Services {
    const backend = new MockBackend();
    const monitoring = new MockMonitoring(backend);
    return {
      routes: new MockRoutesService(backend),
      connections: new MockConnectionsService(backend),
      deadLetters: new MockDeadLettersService(backend),
      activity: new MockActivityService(backend),
      audit: new MockAuditService(backend),
      library: new MockLibraryService(backend),
      samples: new MockSamplesService(backend),
      preview: new MockPreviewService(backend),
      alerts: monitoring.alerts,
      metrics: monitoring.metrics,
      push: monitoring.push,
      liveTail: new MockLiveTailService(backend),
      onboarding: new MockOnboardingService(backend),
      members: new MockMembersService(backend),
      access: new MockAccessService(backend),
      teams: new MockTeamsService(backend),
      approvals: new MockApprovalsService(backend),
      assist: new MockAssistService(backend),
      testCases: new MockTestCasesService(backend),
      runtime: new MockRuntimeService(backend),
      sandbox: new MockSandboxService(backend),
      apiTokens: new MockApiTokensService(backend),
      clock: backend.clock,
    };
  }
}
