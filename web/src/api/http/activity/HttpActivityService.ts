import { ActivityFeed, ActivityVerb, type Activity } from '../../../domain';
import { ActivityService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { ActivityEndpoint, ActivityParam, ActivityVerbDto, type ActivityItemDto, type ActivityPageDto } from './ActivityDtos';

/** `services.activity` over `/api/v1/activity`: the latest route changes from the audit log. */
export class HttpActivityService extends ActivityService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<Activity[]> {
    const page = await this.context.api.get<ActivityPageDto>(ActivityEndpoint.collection, { [ActivityParam.limit]: ActivityFeed.recentLimit });
    return page.items.map((item) => ActivityMapper.activity(item));
  }
}

/** Maps API verbs (snake_case) onto the feed's display verbs. */
export class ActivityMapper {
  static readonly verbs: Record<ActivityVerbDto, ActivityVerb> = {
    [ActivityVerbDto.Created]: ActivityVerb.Created,
    [ActivityVerbDto.Edited]: ActivityVerb.Edited,
    [ActivityVerbDto.Deployed]: ActivityVerb.Deployed,
    [ActivityVerbDto.Paused]: ActivityVerb.Paused,
    [ActivityVerbDto.Resumed]: ActivityVerb.Resumed,
    [ActivityVerbDto.RolledBack]: ActivityVerb.RolledBack,
  };

  static activity(dto: ActivityItemDto): Activity {
    return { id: dto.id, actor: dto.actor, verb: ActivityMapper.verbs[dto.verb] ?? ActivityVerb.Edited, routeName: dto.routeName, detail: dto.detail, at: dto.at };
  }
}
