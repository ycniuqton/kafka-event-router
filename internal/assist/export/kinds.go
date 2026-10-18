package export

import (
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// KindTemplate holds the statement templates of each recipe writer.
type KindTemplate string

const (
	TplMove        KindTemplate = "{ const r = path.get(event, %s); if (r.ok) { path.del(event, %s); path.set(event, %s, r.v); } }"
	TplCopy        KindTemplate = "{ const r = path.get(event, %s); if (r.ok) path.set(event, %s, path.clone(r.v)); }"
	TplRemove      KindTemplate = "path.del(event, %s);"
	TplKeepOnly    KindTemplate = "{ const kept = { value: {} }; for (const segs of [%s]) { const r = path.get(event, segs); if (r.ok) path.set(kept, segs, r.v); } event.value = kept.value; }"
	TplSet         KindTemplate = "path.set(event, %s, v);"
	TplDefault     KindTemplate = "{ const r = path.get(event, %s); if (!r.ok || r.v === null) path.set(event, %s, %s); }"
	TplSetKey      KindTemplate = "event.key = v;"
	TplMask        KindTemplate = "{ const r = path.get(event, %s); if (r.ok) path.set(event, %s, r.v === null ? null : ctx.mask.%s(r.v)); }"
	TplHash        KindTemplate = "{ const r = path.get(event, %s); if (r.ok && r.v !== null) path.set(event, %s, ctx.hash(r.v)); }"
	ListComma      KindTemplate = ", "
	StatementBreak KindTemplate = "\n  "
)

// MaskHelper maps mask styles to the ctx.mask helper names of the sandbox.
var MaskHelper = map[config.MaskStyle]string{
	config.MaskStyleEmail: "email",
	config.MaskStyleLast4: "last4",
	config.MaskStyleFull:  "full",
}

type relocateWriter struct {
	kind       config.StepKind
	keepSource bool
}

func (w relocateWriter) Kind() config.StepKind { return w.kind }
func (w relocateWriter) Write(s config.Step) (string, *WriteError) {
	from, err := JsPath{}.Of(s.From, config.TokenFrom)
	if err != nil {
		return "", err
	}
	to, err := JsPath{}.Of(s.To, config.TokenTo)
	if err != nil {
		return "", err
	}
	if w.keepSource {
		return fmt.Sprintf(string(TplCopy), from, to), nil
	}
	return fmt.Sprintf(string(TplMove), from, from, to), nil
}

type removeWriter struct{}

func (removeWriter) Kind() config.StepKind { return config.StepKindRemove }
func (removeWriter) Write(s config.Step) (string, *WriteError) {
	paths, err := FieldPaths{}.Of(s.Fields)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(paths))
	for _, p := range paths {
		lines = append(lines, fmt.Sprintf(string(TplRemove), p))
	}
	return Joiner{}.Lines(lines), nil
}

type keepOnlyWriter struct{}

func (keepOnlyWriter) Kind() config.StepKind { return config.StepKindKeepOnly }
func (keepOnlyWriter) Write(s config.Step) (string, *WriteError) {
	paths, err := FieldPaths{}.Of(s.Fields)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(string(TplKeepOnly), Joiner{}.List(paths)), nil
}

type setWriter struct{}

func (setWriter) Kind() config.StepKind { return config.StepKindSet }
func (setWriter) Write(s config.Step) (string, *WriteError) {
	target, err := JsPath{}.Of(s.Field, config.TokenField)
	if err != nil {
		return "", err
	}
	return SourceExpr{}.Wrap(s.Source, fmt.Sprintf(string(TplSet), target))
}

type defaultWriter struct{}

func (defaultWriter) Kind() config.StepKind { return config.StepKindDefaultIfMissing }
func (defaultWriter) Write(s config.Step) (string, *WriteError) {
	target, err := JsPath{}.Of(s.Field, config.TokenField)
	if err != nil {
		return "", err
	}
	lit, err := JsLiteral{}.Of(s.Value, config.TokenValue)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(string(TplDefault), target, target, lit), nil
}

type setKeyWriter struct{}

func (setKeyWriter) Kind() config.StepKind { return config.StepKindSetKey }
func (setKeyWriter) Write(s config.Step) (string, *WriteError) {
	return SourceExpr{}.Wrap(s.Source, string(TplSetKey))
}

type maskWriter struct{}

func (maskWriter) Kind() config.StepKind { return config.StepKindMask }
func (maskWriter) Write(s config.Step) (string, *WriteError) {
	target, err := JsPath{}.Of(s.Field, config.TokenField)
	if err != nil {
		return "", err
	}
	if s.Style == nil {
		return "", MemberError(config.TokenStyle, string(MsgMissingMember))
	}
	helper, ok := MaskHelper[*s.Style]
	if !ok {
		return "", MemberError(config.TokenStyle, string(MsgStyleUnsupported))
	}
	return fmt.Sprintf(string(TplMask), target, target, helper), nil
}

type hashWriter struct{}

func (hashWriter) Kind() config.StepKind { return config.StepKindHash }
func (hashWriter) Write(s config.Step) (string, *WriteError) {
	target, err := JsPath{}.Of(s.Field, config.TokenField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(string(TplHash), target, target), nil
}

// FieldPaths renders a fields list, pointing failures at the failing index.
type FieldPaths struct{}

// Of renders every path of fields.
func (FieldPaths) Of(fields []string) ([]string, *WriteError) {
	out := make([]string, 0, len(fields))
	for i := range fields {
		p, err := JsPath{}.Of(&fields[i], config.TokenFields)
		if err != nil {
			err.At = config.PointerRoot.Token(config.TokenFields).Index(i)
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
