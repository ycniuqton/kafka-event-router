import type { Diagnostic, RuntimeInfo, SandboxEntry } from '../../domain';
import { SANDBOX_RUNTIME_INFO } from '../../mocks';
import { SandboxService } from '../contracts';
import type { MockBackend } from './MockBackend';
import { MockCodeChecker } from './MockCodeChecker';

/** Mock `services.sandbox`: the runtime info of a default-configured API and a parse-only code check. */
export class MockSandboxService extends SandboxService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  runtimeInfo(): Promise<RuntimeInfo> {
    return this.backend.reply(SANDBOX_RUNTIME_INFO);
  }

  check(code: string, entry: SandboxEntry): Promise<Diagnostic[]> {
    return this.backend.reply(MockCodeChecker.check(code, entry));
  }
}
