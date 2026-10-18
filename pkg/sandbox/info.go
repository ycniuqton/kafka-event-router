package sandbox

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// HelperDoc documents one ctx helper.
type HelperDoc = host.HelperDoc

// SafetyRuleID names a safety rule shown in the JS editor info panel.
type SafetyRuleID string

const (
	SafetyIsolation   SafetyRuleID = "isolation"
	SafetyLimits      SafetyRuleID = "limits"
	SafetyDeterminism SafetyRuleID = "determinism"
	SafetyOutput      SafetyRuleID = "output"
	SafetyVersions    SafetyRuleID = "versions"
)

// SafetyRule is one user-facing safety statement (UI spec §8.8).
type SafetyRule struct {
	ID   SafetyRuleID `json:"id"`
	Text string       `json:"text"`
}

// SafetyRules are the texts of UI spec §8.8, enforced by the sandbox and the platform.
var SafetyRules = []SafetyRule{
	{SafetyIsolation, "Code runs in an isolated sandbox: no network calls, no file access, no access to credentials or other routes."},
	{SafetyLimits, "Time limit per event and memory limit per route; exceeding them sends the event to the dead-letter topic and shows a warning on the step."},
	{SafetyDeterminism, "No randomness or current time except through ctx, so test cases are repeatable (Math.random and Date are seeded and frozen per event)."},
	{SafetyOutput, "Output must be valid for the destination format; if a destination schema is set, output is validated against it before sending."},
	{SafetyVersions, "Every transform version is kept, and the route detail shows which version produced which output."},
}

// LimitsDoc is the limits block of runtime-info in route-document units.
type LimitsDoc struct {
	JSTimeoutMs   int64 `json:"jsTimeoutMs"`
	JSMemoryMb    int64 `json:"jsMemoryMb"`
	JSMaxOutputKb int64 `json:"jsMaxOutputKb"`
	LogKbPerCall  int64 `json:"logKbPerCall,omitempty"`
}

// LimitsInfo pairs defaults and operator maxima.
type LimitsInfo struct {
	Defaults LimitsDoc `json:"defaults"`
	Maxima   LimitsDoc `json:"maxima"`
}

// RuntimeInfo is served by GET /api/v1/sandbox/runtime-info.
type RuntimeInfo struct {
	Engine             EngineKind          `json:"engine"`
	DeterminismVersion determinism.Version `json:"determinismVersion"`
	Limits             LimitsInfo          `json:"limits"`
	Helpers            []HelperDoc         `json:"helpers"`
	Typings            string              `json:"typings"`
	SafetyRules        []SafetyRule        `json:"safetyRules"`
}

// InfoBuilder assembles RuntimeInfo from the helper catalogue and limit policy.
type InfoBuilder struct{}

// Build returns the runtime-info document.
func (InfoBuilder) Build(engine EngineKind, policy LimitPolicy) RuntimeInfo {
	docs := host.NewStandardRegistry(nil).Docs()
	return RuntimeInfo{
		Engine:             engine,
		DeterminismVersion: determinism.VersionCurrent,
		Limits:             LimitsInfo{Defaults: LimitsDocOf(policy.Defaults, true), Maxima: LimitsDocOf(policy.Maxima, false)},
		Helpers:            docs,
		Typings:            TypingsBuilder{}.Build(docs),
		SafetyRules:        SafetyRules,
	}
}

// LimitsDocOf converts Limits to document units; withLog adds the per-call log cap.
func LimitsDocOf(l Limits, withLog bool) LimitsDoc {
	d := LimitsDoc{
		JSTimeoutMs:   int64(l.Timeout / time.Millisecond),
		JSMemoryMb:    l.MemoryBytes / int64(MiB),
		JSMaxOutputKb: l.MaxOutputBytes / int64(KiB),
	}
	if withLog {
		d.LogKbPerCall = l.MaxLogBytes / int64(KiB)
	}
	return d
}

// TypingsBuilder renders TypeScript declarations for the Monaco editor.
type TypingsBuilder struct{}

// TypingsHeader declares the event and output shapes of the boundary (design §2.4).
const TypingsHeader TypingsText = `/** Integer beyond ±2^53 on Avro/Protobuf routes, as decimal digits (use BigInt to compute). */
type RouterLong = { $long: string };
/** Binary data (Avro/Protobuf bytes, non-UTF-8 headers), base64-encoded. */
type RouterBytes = { $bytes: string };
/** Exact decimal on Avro/Protobuf routes, e.g. "12.34". */
type RouterDecimal = { $decimal: string };
/** Timestamp on Avro/Protobuf routes, RFC 3339 in UTC. */
type RouterTimestamp = { $timestamp: string };
/** Lossless tagged values of Avro/Protobuf routes; return them unchanged or build new ones. */
type RouterTagged = RouterLong | RouterBytes | RouterDecimal | RouterTimestamp;
type RouterHeaderValue = string | RouterBytes;
interface RouterEvent {
  key: unknown;
  value: any;
  headers: Record<string, RouterHeaderValue>;
  topic: string;
  partition: number;
  offset: number;
  timestamp: string;
}
interface RouterOutput {
  key?: unknown;
  value?: unknown;
  headers?: Record<string, RouterHeaderValue>;
}
`

// TypingsFooter declares the globals visible to user code.
const TypingsFooter TypingsText = `declare const ctx: RouterCtx;
declare function transform(event: RouterEvent, ctx: RouterCtx): RouterOutput | RouterOutput[] | null;
`

// TypingsText is a fragment of generated declarations.
type TypingsText string

// Build renders RouterCtx from helper docs; dotted names become nested groups.
func (TypingsBuilder) Build(docs []HelperDoc) string {
	groups := map[string][]HelperDoc{}
	var top []HelperDoc
	for _, d := range docs {
		group, member, nested := strings.Cut(string(d.Name), ".")
		if nested {
			groups[group] = append(groups[group], HelperDoc{Name: host.FnName(member), Signature: d.Signature, Doc: d.Doc})
			continue
		}
		top = append(top, d)
	}
	var b strings.Builder
	b.WriteString(string(TypingsHeader))
	b.WriteString("interface RouterCtx {\n")
	TypingsMembers{indent: "  "}.write(&b, top)
	TypingsGroups{}.write(&b, groups)
	b.WriteString("}\n")
	b.WriteString(string(TypingsFooter))
	return b.String()
}

// TypingsMembers writes documented members at one indentation level.
type TypingsMembers struct {
	indent string
}

func (t TypingsMembers) write(b *strings.Builder, docs []HelperDoc) {
	for _, d := range docs {
		fmt.Fprintf(b, "%s/** %s */\n%s%s: %s;\n", t.indent, d.Doc, t.indent, d.Name, d.Signature)
	}
}

// TypingsGroups writes nested helper groups such as ctx.mask in name order.
type TypingsGroups struct{}

func (TypingsGroups) write(b *strings.Builder, groups map[string][]HelperDoc) {
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(b, "  %s: {\n", n)
		TypingsMembers{indent: "    "}.write(b, groups[n])
		b.WriteString("  };\n")
	}
}
