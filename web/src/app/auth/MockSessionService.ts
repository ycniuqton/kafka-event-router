import { AuthMode, SessionService, type AuthConfig, type SessionUser } from '../../api/http/core';
import { AuthPaths } from './AuthPaths';

/** Mock data source: always signed in as the demo admin, so the mock app never shows the login screen. */
export class MockSessionService extends SessionService {
  static readonly user: SessionUser = {
    id: '00000000-0000-0000-0000-000000000001',
    email: 'dev@eventrouter.local',
    display_name: 'Dev Admin',
    initials: 'DA',
    teams: [],
    workspace: 'Demo',
    csrf_token: '',
  };

  me(): Promise<SessionUser> {
    return Promise.resolve(MockSessionService.user);
  }

  config(): Promise<AuthConfig> {
    return Promise.resolve({ modes: [AuthMode.Dev], workspace: MockSessionService.user.workspace });
  }

  devLogin(): Promise<void> {
    return Promise.resolve();
  }

  logout(): Promise<void> {
    return Promise.resolve();
  }

  oidcLoginUrl(): string {
    return AuthPaths.home;
  }
}
