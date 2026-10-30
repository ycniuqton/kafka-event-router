import { describe, expect, it } from 'vitest';
import { HttpContext } from '../HttpContext';
import { TestFetch } from '../TestFetch.test-util';
import { HttpSessionService } from './HttpSessionService';
import { AuthProblemCode } from './SessionService';

const me = { id: 'u1', email: 'a@x.io', display_name: 'A', initials: 'A', teams: [], workspace: 'w', csrf_token: 'fresh-csrf', must_change_password: false };

function session(fetch: TestFetch): { svc: HttpSessionService; context: HttpContext } {
  const context = new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn });
  return { svc: new HttpSessionService(context), context };
}

describe('HttpSessionService (password mode)', () => {
  it('posts the credentials to password_login_url', async () => {
    const fetch = new TestFetch().empty(204);
    await session(fetch).svc.passwordLogin({ email: 'a@x.io', password: 'secret-password' }, '/auth/password/login');
    expect(fetch.calls[0].url).toBe('/auth/password/login');
    expect(fetch.calls[0].init.method).toBe('POST');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ email: 'a@x.io', password: 'secret-password' });
  });

  it('reads Retry-After of a locked login', async () => {
    const fetch = new TestFetch().json(429, { type: 'https://eventrouter.dev/problems/rate-limited', title: 'Too many', status: 429, code: AuthProblemCode.LoginLocked }, { 'Retry-After': '120' });
    const error = await TestFetch.failure(session(fetch).svc.passwordLogin({ email: 'a@x.io', password: 'p' }));
    expect(error.code).toBe(AuthProblemCode.LoginLocked);
    expect(error.retryAfter).toBe(120);
  });

  it('changes the password with CSRF, then re-reads /auth/me for the rotated CSRF token', async () => {
    const fetch = new TestFetch().empty(204).json(200, me);
    const { svc, context } = session(fetch);
    context.csrf.set('old-csrf');
    const user = await svc.changePassword({ current_password: 'old-password', new_password: 'new-password-123' });
    expect(fetch.calls[0].url).toBe('/api/v1/auth/password');
    expect(fetch.header(0, 'X-CSRF-Token')).toBe('old-csrf');
    expect(fetch.calls[1].url).toBe('/api/v1/auth/me');
    expect(user.csrf_token).toBe('fresh-csrf');
    expect(context.csrf.current()).toBe('fresh-csrf');
  });
});
