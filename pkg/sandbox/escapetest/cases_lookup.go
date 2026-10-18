package escapetest

import "github.com/eventrouter/router/pkg/sandbox"

var (
	hostError = FailsWith{Kinds: []sandbox.ErrorKind{sandbox.ErrorHost}}
	null      = Returns{Value: nil}
)

// HostRaceAttack keeps the guest inside host calls with large arguments so the deadline often
// fires while the host reads guest memory (regression: the host must never touch freed memory).
const HostRaceAttack = "for (;;) { ctx.lookup('countries', 'k'); ctx.hash('x'.repeat(20000)); }"

// LookupCases abuse ctx.lookup (plan JS-206): oversized arguments throw a catchable host error,
// unknown tables and odd keys read null, floods hit the time limit; the next event is healthy.
func LookupCases() []Case {
	hugeKey := "'k'.repeat(1024 * 1024)"
	return []Case{
		{Name: "lookup huge key", Limits: generousClock, Code: attackThenHealthy("return { value: ctx.lookup('countries', " + hugeKey + ") };"),
			Attack: hostError, After: healthy},
		{Name: "lookup huge key caught", Limits: generousClock,
			Code:   attackThenHealthy("try { ctx.lookup('countries', " + hugeKey + "); } catch (e) { return { value: 'caught' }; }"),
			Attack: Returns{Value: "caught"}, After: healthy},
		{Name: "lookup huge table name", Code: attackThenHealthy("return { value: ctx.lookup('t'.repeat(100000), 'DE') };"),
			Attack: hostError, After: healthy},
		{Name: "lookup non-string table", Code: attackThenHealthy("return { value: ctx.lookup(42, 'DE') };"), Attack: hostError, After: healthy},
		{Name: "lookup missing key", Code: attackThenHealthy("return { value: ctx.lookup('countries') };"), Attack: null, After: healthy},
		{Name: "lookup missing table", Code: attackThenHealthy("return { value: ctx.lookup('no-such-table', 'DE') };"), Attack: null, After: healthy},
		{Name: "lookup prototype keys", Code: attackThenHealthy("return { value: ['__proto__', 'constructor', 'toString', 'hasOwnProperty'].map((k) => ctx.lookup('countries', k)) };"),
			Attack: Returns{Value: []any{nil, nil, nil, nil}}, After: healthy},
		{Name: "lookup object key", Code: attackThenHealthy("return { value: ctx.lookup('countries', { toString() { return 'DE'; } }) };"),
			Attack: null, After: healthy},
		{Name: "lookup flood", Code: attackThenHealthy("for (;;) ctx.lookup('countries', 'DE');"), Attack: timeout, After: healthy},
		{Name: "host calls racing the deadline", Code: attackThenHealthy(HostRaceAttack), Attack: timeout, After: healthy},
		{Name: "lookup result mutation", Code: "function transform(event, ctx) {\n  if (event.offset === 1) { const v = ctx.lookup('countries', 'DE'); return { value: v + '!' }; }\n  return { value: ctx.lookup('countries', 'DE') };\n}",
			Attack: Returns{Value: "Germany!"}, After: Returns{Value: "Germany"}},
	}
}
