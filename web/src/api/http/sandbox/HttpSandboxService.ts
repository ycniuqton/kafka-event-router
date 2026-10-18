import type { Diagnostic, RuntimeInfo, SandboxEntry, SandboxLimits } from '../../../domain';
import { SandboxService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import {
  SandboxEndpoint,
  type CheckRequestDto,
  type CheckResponseDto,
  type DiagnosticDto,
  type RuntimeInfoDto,
  type SandboxLimitsDto,
} from './SandboxDtos';

/** `services.sandbox` over `/api/v1/sandbox/*`: runtime info is static per deployment (read once per page load); checks are not cached. */
export class HttpSandboxService extends SandboxService {
  private readonly context: HttpContext;
  private cached: Promise<RuntimeInfo> | null = null;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  runtimeInfo(): Promise<RuntimeInfo> {
    this.cached ??= this.load();
    return this.cached;
  }

  async check(code: string, entry: SandboxEntry): Promise<Diagnostic[]> {
    const body: CheckRequestDto = { code, entry };
    const dto = await this.context.api.post<CheckResponseDto>(SandboxEndpoint.check, body);
    return (dto.diagnostics ?? []).map(SandboxMapper.diagnostic);
  }

  private async load(): Promise<RuntimeInfo> {
    try {
      return SandboxMapper.info(await this.context.api.get<RuntimeInfoDto>(SandboxEndpoint.runtimeInfo));
    } catch (error) {
      this.cached = null;
      throw error;
    }
  }
}

/** Maps the runtime-info DTO (optional members omitted by the server) to the domain value. */
export class SandboxMapper {
  static info(dto: RuntimeInfoDto): RuntimeInfo {
    return {
      engine: dto.engine,
      determinismVersion: dto.determinismVersion,
      limits: { defaults: SandboxMapper.limits(dto.limits.defaults), maxima: SandboxMapper.limits(dto.limits.maxima) },
      helpers: dto.helpers ?? [],
      typings: dto.typings,
      safetyRules: dto.safetyRules ?? [],
    };
  }

  static diagnostic(dto: DiagnosticDto): Diagnostic {
    return { severity: dto.severity, message: dto.message, line: dto.line ?? null, column: dto.column ?? null, path: dto.path ?? null };
  }

  static limits(dto: SandboxLimitsDto): SandboxLimits {
    return { ...dto, logKbPerCall: dto.logKbPerCall ?? null };
  }
}
