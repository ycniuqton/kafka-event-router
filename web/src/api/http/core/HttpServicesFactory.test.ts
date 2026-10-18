import { describe, expect, it, vi } from 'vitest';
import type { Services } from '../../contracts';
import { HttpLibraryService } from '../library/HttpLibraryService';
import { HttpContext } from './HttpContext';
import { HttpServiceModule, ServiceKeys } from './HttpServiceModule';
import { HttpServicesFactory } from './HttpServicesFactory';
import { HttpMembersService } from './session/HttpMembersModule';
import { TestFetch } from './TestFetch.test-util';

class EmptyLibrary extends HttpLibraryService {
  constructor() {
    super(new HttpContext({ fetch: new TestFetch().fn }));
  }
}

class LibraryModule extends HttpServiceModule<typeof ServiceKeys.Library> {
  readonly key = ServiceKeys.Library;
  create(): Services[typeof ServiceKeys.Library] {
    return new EmptyLibrary();
  }
}

describe('HttpServicesFactory', () => {
  it('uses provided modules and warns only about keys without one', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    HttpServicesFactory.provide(new LibraryModule());
    const services = HttpServicesFactory.create(new HttpContext({ fetch: new TestFetch().fn }));
    expect(services.library).toBeInstanceOf(EmptyLibrary);
    expect(services.members).toBeInstanceOf(HttpMembersService);
    expect(typeof services.routes.list).toBe('function');
    const warned = warn.mock.calls.map((call) => String(call[1])).join(',');
    expect(warned).not.toContain(ServiceKeys.Library);
    expect(warned).not.toContain(ServiceKeys.Audit);
    warn.mockRestore();
  });

  it('maps /auth/me onto the current user and keeps the CSRF token', async () => {
    const fetch = new TestFetch().json(200, {
      id: 'u1', email: 'ada@x.io', display_name: 'Ada Lovelace', initials: 'AL', workspace: 'Acme', csrf_token: 'tok',
      teams: [{ id: 't1', name: 'payments', role: 'editor' }, { id: 't2', name: 'ops', role: 'admin' }],
    });
    const context = new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn });
    const user = await HttpServicesFactory.create(context).members.current();
    expect(fetch.calls[0].url).toBe('/api/v1/auth/me');
    expect(user).toEqual({ name: 'Ada Lovelace', initials: 'AL', workspace: 'Acme', team: 'payments', role: 'Admin' });
    expect(context.csrf.current()).toBe('tok');
  });
});
