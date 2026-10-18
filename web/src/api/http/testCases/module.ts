import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpTestCasesService } from './HttpTestCasesService';

/** REST module for `services.testCases` (auto-discovered by HttpServicesFactory). */
export class HttpTestCasesModule extends HttpServiceModule<typeof ServiceKeys.TestCases> {
  readonly key = ServiceKeys.TestCases;

  create(context: HttpContext): Services[typeof ServiceKeys.TestCases] {
    return new HttpTestCasesService(context);
  }
}

export default new HttpTestCasesModule();
