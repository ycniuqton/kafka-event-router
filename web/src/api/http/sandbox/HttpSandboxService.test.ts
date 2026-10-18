import { describe, expect, it } from 'vitest';
import { SandboxSummary } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpSandboxService } from './HttpSandboxService';

const body = {
  engine: 'quickjs_wazero',
  determinismVersion: 1,
  limits: {
    defaults: { jsTimeoutMs: 40, jsMemoryMb: 64, jsMaxOutputKb: 1024, logKbPerCall: 16 },
    maxima: { jsTimeoutMs: 1000, jsMemoryMb: 256, jsMaxOutputKb: 8192 },
  },
  helpers: [{ name: 'hash', signature: '(value: unknown) => string', doc: 'Salted hash.' }],
  typings: 'declare const ctx: unknown;',
  safetyRules: [{ id: 'isolation', text: 'No network.' }],
};

function setup() {
  const fetch = new TestFetch();
  return { fetch, service: new HttpSandboxService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })) };
}

describe('HttpSandboxService', () => {
  it('reads runtime info once and maps omitted limits to null', async () => {
    const { fetch, service } = setup();
    fetch.json(200, body);
    const info = await service.runtimeInfo();
    await service.runtimeInfo();
    expect(fetch.calls).toHaveLength(1);
    expect(fetch.calls[0].url).toContain('/api/v1/sandbox/runtime-info');
    expect(info.limits.maxima.logKbPerCall).toBeNull();
    expect(info.helpers[0].name).toBe('hash');
    expect(SandboxSummary.badge(info)).toBe('Sandboxed · no network · 40 ms / event');
  });

  it('retries after a failed read', async () => {
    const { fetch, service } = setup();
    fetch.problem(500, { title: 'boom', status: 500 }).json(200, { ...body, helpers: null, safetyRules: null });
    await expect(service.runtimeInfo()).rejects.toBeTruthy();
    const info = await service.runtimeInfo();
    expect(info.helpers).toEqual([]);
    expect(info.safetyRules).toEqual([]);
  });
});

describe('HttpSandboxService.check', () => {
  it('posts code and entry and maps omitted positions to null', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      diagnostics: [
        { severity: 'error', message: 'SyntaxError: unexpected token', line: 2, column: 9 },
        { severity: 'error', message: 'transform(event, ctx) is not defined' },
      ],
    });
    const diags = await service.check('function nope() {\n  let x = ;\n}', 'transform');
    expect(fetch.calls[0].url).toContain('/api/v1/sandbox/check');
    expect(fetch.calls[0].init?.method).toBe('POST');
    expect(JSON.parse(String(fetch.calls[0].init?.body))).toEqual({ code: 'function nope() {\n  let x = ;\n}', entry: 'transform' });
    expect(diags[0]).toEqual({ severity: 'error', message: 'SyntaxError: unexpected token', line: 2, column: 9, path: null });
    expect(diags[1].line).toBeNull();
  });

  it('treats a null list as no diagnostics and surfaces problems', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { diagnostics: null }).problem(413, { title: 'too large', status: 413 });
    await expect(service.check('1', 'predicate')).resolves.toEqual([]);
    await expect(service.check('x'.repeat(10), 'transform')).rejects.toBeTruthy();
  });
});
