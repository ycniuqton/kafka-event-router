import type { RouteRuntime, WorkerInfo } from '../../../domain';
import { RuntimeService } from '../../contracts';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import type { HttpContext } from '../core/HttpContext';
import { RuntimeEndpoint, type RouteRuntimeDto, type WorkerDto } from './RuntimeDtos';

/** `services.runtime` over `GET /api/v1/routes/{id}/runtime` and `GET /api/v1/workers`. */
export class HttpRuntimeService extends RuntimeService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async get(routeId: string): Promise<RouteRuntime> {
    return RuntimeMapper.runtime(await this.context.api.get<RouteRuntimeDto>(RuntimeEndpoint.route(routeId)));
  }

  async workers(): Promise<WorkerInfo[]> {
    const pages = new CursorIterator<WorkerDto>(this.context.api, RuntimeEndpoint.workers, { limit: PageLimit.Max });
    return (await pages.all()).map((dto) => RuntimeMapper.worker(dto));
  }
}

/** Maps runtime DTOs (optional members omitted by the server) to domain values. */
export class RuntimeMapper {
  static runtime(dto: RouteRuntimeDto): RouteRuntime {
    return {
      observedState: dto.observedState,
      runningVersionNo: dto.runningVersionNo ?? null,
      haltedAt: dto.haltedAt ?? null,
      haltReason: dto.haltReason ?? null,
      errorMessage: dto.errorMessage ?? null,
      lag: dto.lag,
      replicas: dto.replicas.map((r) => ({ ...r, versionNo: r.versionNo ?? null, reportedAt: r.reportedAt ?? null, errorMessage: r.errorMessage ?? null })),
    };
  }

  static worker(dto: WorkerDto): WorkerInfo {
    return { ...dto };
  }
}
