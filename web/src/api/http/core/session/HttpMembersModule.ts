import { MembersService, type Services } from '../../../contracts';
import type { CurrentUser, Member } from '../../../../domain';
import { HttpAccessService } from '../../members/HttpAccessService';
import type { HttpContext } from '../HttpContext';
import { HttpServiceModule, ServiceKeys } from '../HttpServiceModule';
import { CurrentUserMapper } from './CurrentUserMapper';
import { HttpSessionService } from './HttpSessionService';

/** `current()` comes from `/auth/me`; `list()` reads the real member rows (`/users`) via access-control. */
export class HttpMembersService extends MembersService {
  private readonly session: HttpSessionService;
  private readonly access: HttpAccessService;

  constructor(context: HttpContext) {
    super();
    this.session = new HttpSessionService(context);
    this.access = new HttpAccessService(context);
  }

  async list(): Promise<Member[]> {
    return (await this.access.members()).map(({ name, role, team }) => ({ name, role, team }));
  }

  async current(): Promise<CurrentUser> {
    return CurrentUserMapper.toCurrentUser(await this.session.me());
  }
}

/** Default members module of the http data source (access-control may replace it). */
export class HttpMembersModule extends HttpServiceModule<typeof ServiceKeys.Members> {
  readonly key = ServiceKeys.Members;

  create(context: HttpContext): Services[typeof ServiceKeys.Members] {
    return new HttpMembersService(context);
  }
}
