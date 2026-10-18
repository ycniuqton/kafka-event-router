/*
 * Sandbox prelude: runs before user code in every guest instance. It removes engine
 * globals, blocks dynamic code, installs the frozen ctx object, a seeded Math.random and a
 * frozen Date, and returns [invoke, describe, lockdown] to the host.
 */
(function (global) {
  'use strict';
  const OP_INPUT = 'input';
  const OP_HOST = 'host';
  const UINT32_RANGE = 4294967296;
  const REMOVED_GLOBALS = ['__sb_host', 'std', 'os', 'bjson', 'gc', 'print', 'scriptArgs', 'navigator', 'QJS_PROXY_VALUE',
    'console', 'setTimeout', 'setInterval', 'clearTimeout', 'clearInterval', 'queueMicrotask'];
  const MSG_DYNAMIC_CODE = 'dynamic code evaluation is disabled in the sandbox';
  const MSG_UNDEFINED = 'transform returned undefined; return the event, null or an array of events';
  const MSG_NOT_SERIALISABLE = 'output is not JSON-serialisable: ';
  const MSG_ASYNC = 'transform must return synchronously; promises and async functions are not supported';
  const MSG_UNCAUGHT = '{"ok":false,"error":{"kind":"runtime","message":"uncaught exception"}}';

  const host = global.__sb_host;
  const stringify = JSON.stringify;
  const parse = JSON.parse;
  const freeze = Object.freeze;
  const define = Object.defineProperty;
  const protoOf = Object.getPrototypeOf;
  const OrigDate = global.Date;
  const state = { nowMs: 0, a: 0, b: 0, c: 0, d: 0 };

  for (const name of REMOVED_GLOBALS) {
    try { delete global[name]; } catch (_) { define(global, name, { value: undefined }); }
  }

  class HostError extends Error {
    constructor(message) { super(message); this.name = 'HostError'; }
  }

  function call(op, name, args) {
    const reply = parse(host(op, name, stringify(args === undefined ? null : args)));
    if (!reply.ok) throw new HostError(reply.error);
    return reply.value;
  }

  function nextRandom() {
    const t = (((state.a + state.b) | 0) + state.d) | 0;
    state.d = (state.d + 1) | 0;
    state.a = state.b ^ (state.b >>> 9);
    state.b = (state.c + (state.c << 3)) | 0;
    state.c = ((state.c << 21) | (state.c >>> 11)) + t | 0;
    return (t >>> 0) / UINT32_RANGE;
  }

  function reseed(rng) {
    state.a = rng[0] | 0; state.b = rng[1] | 0; state.c = rng[2] | 0; state.d = rng[3] | 0;
  }

  function FrozenDate(...args) {
    if (!new.target) return new OrigDate(state.nowMs).toString();
    return args.length === 0 ? new OrigDate(state.nowMs) : new OrigDate(...args);
  }
  FrozenDate.prototype = OrigDate.prototype;
  FrozenDate.now = function now() { return state.nowMs; };
  FrozenDate.parse = OrigDate.parse;
  FrozenDate.UTC = OrigDate.UTC;
  define(OrigDate.prototype, 'constructor', { value: FrozenDate });
  define(global, 'Date', { value: FrozenDate, writable: false, configurable: false });
  define(Math, 'random', { value: function random() { return nextRandom(); }, writable: false, configurable: false });

  const blocked = function () { throw new EvalError(MSG_DYNAMIC_CODE); };
  blocked.prototype = Function.prototype;
  for (const sample of [function () {}, async function () {}, function* () {}, async function* () {}]) {
    define(protoOf(sample), 'constructor', { value: blocked, writable: false, configurable: false });
  }
  define(global, 'Function', { value: blocked, writable: false, configurable: false });
  define(global, 'eval', { value: blocked, writable: false, configurable: false });

  function text(v) {
    if (typeof v === 'string') return v;
    try { const s = stringify(v); return s === undefined ? String(v) : s; } catch (_) { return String(v); }
  }
  function log(...values) { call(OP_HOST, 'log', [values.map(text).join(' ')]); }

  const ctx = freeze({
    mask: freeze({
      email: (v) => call(OP_HOST, 'mask.email', [v]),
      last4: (v) => call(OP_HOST, 'mask.last4', [v]),
      full: (v) => call(OP_HOST, 'mask.full', [v]),
    }),
    hash: (v) => call(OP_HOST, 'hash', [v]),
    now: () => call(OP_HOST, 'now', []),
    formatDate: (v, layout, tz) => call(OP_HOST, 'formatDate', [v, layout, tz]),
    lookup: (table, key) => call(OP_HOST, 'lookup', [table, key]),
    log: log,
  });
  define(global, 'console', { value: freeze({ log: log, info: log, warn: log, error: log, debug: log }), writable: false, configurable: false });

  function describe(e) {
    const isObj = e !== null && (typeof e === 'object' || typeof e === 'function');
    const name = isObj && e.name ? String(e.name) : 'Error';
    const message = isObj && 'message' in e ? String(e.message) : String(e);
    let kind = 'runtime';
    if (e instanceof HostError) kind = 'host';
    else if (name === 'InternalError' && message.indexOf('out of memory') >= 0) kind = 'memory';
    return { kind: kind, message: name + ': ' + message, stack: isObj && e.stack ? String(e.stack) : '' };
  }

  function describeJSON(e) {
    try { return stringify(describe(e)); } catch (_) { return MSG_UNCAUGHT; }
  }

  function fail(e) {
    try { return stringify({ ok: false, error: describe(e) }); } catch (_) { return MSG_UNCAUGHT; }
  }

  function invalid(message) {
    return stringify({ ok: false, error: { kind: 'invalid_output', message: message } });
  }

  function encode(out) {
    if (out === undefined) return invalid(MSG_UNDEFINED);
    if (out !== null && typeof out === 'object' && typeof out.then === 'function') return invalid(MSG_ASYNC);
    let body;
    try { body = stringify(out); } catch (e) { return invalid(MSG_NOT_SERIALISABLE + describe(e).message); }
    if (body === undefined) return invalid(MSG_UNDEFINED);
    return '{"ok":true,"output":' + body + '}';
  }

  function invoke(fn) {
    let input;
    try {
      input = call(OP_INPUT, '', null);
      state.nowMs = input.nowMs;
      reseed(input.rng);
    } catch (e) { return fail(e); }
    try {
      const out = fn(input.event, ctx);
      if (input.predicate) return stringify({ ok: true, bool: !!out });
      return encode(out);
    } catch (e) { return fail(e); }
  }

  function lockdown() {
    for (const name of Object.getOwnPropertyNames(global)) {
      const v = global[name];
      if (v === null || (typeof v !== 'object' && typeof v !== 'function')) continue;
      try { freeze(v); if (v.prototype) freeze(v.prototype); } catch (_) { continue; }
    }
    return true;
  }

  return [invoke, describeJSON, lockdown];
})(globalThis);
