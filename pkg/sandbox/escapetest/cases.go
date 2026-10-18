package escapetest

import "github.com/eventrouter/router/pkg/sandbox"

// Case is one escape attempt: code runs on event offset 1 (Attack) and then offset 2 (After)
// on the same module, so leaks into the next event are observable.
type Case struct {
	Name   string
	Code   string
	Limits sandbox.RouteLimits
	Attack Expectation
	After  Expectation
}

var (
	timeout       = FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorTimeout}}
	memory        = FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorMemory}}
	runtimeError  = FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorRuntime}}
	healthy       = Returns{Value: "healthy"}
	smallMemory   = sandbox.RouteLimits{JSMemoryMb: 16, JSTimeoutMs: 1000}
	generousClock = sandbox.RouteLimits{JSTimeoutMs: 1000}
)

// attackThenHealthy wraps an attack so that offset 2 returns "healthy".
func attackThenHealthy(attack string) string {
	return "function transform(event, ctx) {\n  if (event.offset === 2) return { value: 'healthy' };\n" + attack + "\n}"
}

// Cases is the MVP escape suite (plan JS-44).
func Cases() []Case {
	return append(append(resourceCases(), leakCases()...), accessCases()...)
}

func resourceCases() []Case {
	return []Case{
		{Name: "infinite loop", Code: attackThenHealthy("while (true) {}"), Attack: timeout, After: healthy},
		{Name: "deep recursion", Code: attackThenHealthy("const f = (n) => f(n + 1) + 1; return f(0);"),
			Attack: FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorStackOverflow}}, After: healthy},
		{Name: "array memory bomb", Limits: smallMemory, Code: attackThenHealthy("const a = []; for (;;) a.push(new Array(10000).fill(1));"),
			Attack: memory, After: healthy},
		{Name: "string memory bomb", Limits: smallMemory, Code: attackThenHealthy("let s = 'x'; for (;;) s = s + s;"),
			Attack: FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorMemory, sandbox.ErrorRuntime}}, After: healthy},
		{Name: "huge output", Limits: generousClock, Code: attackThenHealthy("return { value: 'x'.repeat(2 * 1024 * 1024) };"),
			Attack: FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorOutputTooLarge}}, After: healthy},
		{Name: "huge host argument", Limits: generousClock, Code: attackThenHealthy("return { value: ctx.hash('x'.repeat(2 * 1024 * 1024)) };"),
			Attack: FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorHost}}, After: healthy},
		{Name: "pending promise jobs never run", Code: attackThenHealthy("Promise.resolve().then(() => { for (;;) {} }); return { value: 'done' };"),
			Attack: Returns{Value: "done"}, After: healthy},
	}
}

func leakCases() []Case {
	probe := "function transform(event, ctx) {\n  if (event.offset === 1) { %s; return { value: 'attacked' }; }\n  return { value: %s };\n}"
	return []Case{
		{Name: "prototype pollution", Code: sprintf(probe, "Object.prototype.polluted = 'yes'; Array.prototype.push = null", "[({}).polluted === undefined, typeof [].push]"),
			Attack: Returns{Value: "attacked"}, After: Returns{Value: []any{true, "function"}}},
		{Name: "globalThis tampering", Code: sprintf(probe, "globalThis.JSON = null; globalThis.counter = 1; Math.max = () => 0", "[typeof JSON.stringify, typeof counter, Math.max(1, 2)]"),
			Attack: Returns{Value: "attacked"}, After: Returns{Value: []any{"function", "undefined", 2.0}}},
		{Name: "module-level state", Code: "let calls = 0;\nfunction transform(event) { calls++; return { value: calls }; }",
			Attack: Returns{Value: 1.0}, After: Returns{Value: 1.0}},
		{Name: "ctx monkey-patching", Code: attackThenHealthy("ctx.hash = () => 'forged';"), Attack: runtimeError, After: healthy},
		{Name: "ctx redefinition", Code: attackThenHealthy("Object.defineProperty(ctx.mask, 'email', { value: (v) => v });"), Attack: runtimeError, After: healthy},
		{Name: "Math.random override", Code: attackThenHealthy("Math.random = () => 4;"), Attack: runtimeError, After: healthy},
		{Name: "Date override", Code: attackThenHealthy("Date = function () {};"), Attack: runtimeError, After: healthy},
	}
}

func accessCases() []Case {
	globals := "['std','os','bjson','gc','print','scriptArgs','navigator','require','process','fetch','XMLHttpRequest','WebSocket','setTimeout','setInterval','clearTimeout','queueMicrotask','__sb_host','QJS_PROXY_VALUE'].filter((n) => typeof globalThis[n] !== 'undefined')"
	return []Case{
		{Name: "eval", Code: attackThenHealthy("return { value: eval('1 + 1') };"), Attack: runtimeError, After: healthy},
		{Name: "indirect eval", Code: attackThenHealthy("return { value: (0, globalThis['ev' + 'al'])('1') };"), Attack: runtimeError, After: healthy},
		{Name: "Function constructor", Code: attackThenHealthy("return { value: (function () {}).constructor('return this')() };"), Attack: runtimeError, After: healthy},
		{Name: "async Function constructor", Code: attackThenHealthy("return { value: (async () => {}).constructor('return 1') };"), Attack: runtimeError, After: healthy},
		{Name: "generator Function constructor", Code: attackThenHealthy("return { value: (function* () {}).constructor('yield 1') };"), Attack: runtimeError, After: healthy},
		{Name: "engine and platform globals", Code: attackThenHealthy("return { value: " + globals + " };"), Attack: Returns{Value: []any{}}, After: healthy},
		{Name: "WASI file system via qjs:os", Code: wasiProbe, Attack: Returns{Value: []any{true, true, true}}, After: Returns{Value: []any{true, true, true}}},
		{Name: "WASI env and args via qjs:std", Code: envProbe, Attack: Returns{Value: []any{true, true}}, After: Returns{Value: []any{true, true}}},
	}
}

// wasiProbe imports qjs:os at load (the only time the job queue runs) and records whether
// opening, listing and statting host paths failed.
const wasiProbe = `globalThis.probe = [false, false, false];
import('qjs:os').then((os) => {
  const fd = os.open('/etc/passwd', os.O_RDONLY);
  const [entries, readErr] = os.readdir('/');
  const [st, statErr] = os.stat('/');
  globalThis.probe = [fd < 0, readErr !== 0, statErr !== 0];
}).catch(() => { globalThis.probe = [true, true, true]; });
function transform() { return { value: globalThis.probe }; }`

// envProbe checks that no environment variables or arguments reach the guest.
const envProbe = `globalThis.probe = [false, false];
import('qjs:std').then((std) => {
  const env = std.getenviron();
  globalThis.probe = [Object.keys(env).length === 0, std.getenv('HOME') === undefined];
}).catch(() => { globalThis.probe = [true, true]; });
function transform() { return { value: globalThis.probe }; }`
