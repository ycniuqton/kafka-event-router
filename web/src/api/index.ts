/** Public API surface of the data-access layer. */
export { services, ServiceRegistry } from './registry';
export { ApiConfig, DataSource } from './config';
export type { Services, DeployResult, SampleField, StepProposal, AssistContext, StepExplanation, InferredMapping, MappingProposal, LibrarySaveOptions } from './contracts';
export {
  RoutesService,
  ConnectionsService,
  DeadLettersService,
  ActivityService,
  AuditService,
  LibraryService,
  SamplesService,
  PreviewService,
  AlertsService,
  MetricsService,
  MembersService,
  AccessService,
  AssistService,
  RuntimeService,
} from './contracts';
