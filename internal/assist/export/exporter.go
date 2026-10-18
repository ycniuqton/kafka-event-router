// Package export converts recipe steps into one equivalent JavaScript step ("Export to JS", FR-15).
package export

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/pkg/config"
)

// JsTemplate holds the fixed parts of the generated source.
type JsTemplate string

const (
	TemplateHeader JsTemplate = `// Exported from %d recipe step(s) by the Transform Studio.
function transform(event, ctx) {
  const path = {
    get(o, segs) {
      let c = o;
      for (const s of segs) {
        if (c === null || typeof c !== "object" || !Object.prototype.hasOwnProperty.call(c, s)) return { ok: false };
        c = c[s];
      }
      return { ok: true, v: c };
    },
    set(o, segs, v) {
      let c = o;
      for (let i = 0; i < segs.length - 1; i++) {
        if (c[segs[i]] === null || typeof c[segs[i]] !== "object") c[segs[i]] = {};
        c = c[segs[i]];
      }
      c[segs[segs.length - 1]] = v;
    },
    del(o, segs) {
      const p = path.get(o, segs.slice(0, -1));
      if (!p.ok || p.v === null || typeof p.v !== "object") return;
      if (Array.isArray(p.v)) p.v.splice(segs[segs.length - 1], 1);
      else delete p.v[segs[segs.length - 1]];
    },
    clone(v) {
      return v === undefined ? v : JSON.parse(JSON.stringify(v));
    },
  };
`
	TemplateStep   JsTemplate = "  // %d. %s\n  %s\n"
	TemplateFooter JsTemplate = "  return event;\n}\n"
	LineBreak      JsTemplate = "\n"
	LineSpace      JsTemplate = " "
)

// IssueMessage holds the texts of export issues.
type IssueMessage string

const (
	MsgKindUnsupported IssueMessage = "%q steps cannot be exported to JavaScript."
	MsgStrict          IssueMessage = "Strict steps cannot be exported; turn strict mode off first."
	MsgNothing         IssueMessage = "Select at least one enabled recipe step to export."
)

// JsExporter writes each enabled step through its StepJsWriter and joins them into one js step.
type JsExporter struct {
	writers   *JsWriterManager
	describer *describe.DescriberManager
}

// NewJsExporter wires the writer registry and the describer used for step comments.
func NewJsExporter(writers *JsWriterManager, describer *describe.DescriberManager) *JsExporter {
	return &JsExporter{writers: writers, describer: describer}
}

// Export returns the js step, or the issues (JSON Pointers under base) that prevent the export.
func (e *JsExporter) Export(base config.Pointer, steps []config.Step) (config.Step, config.Issues) {
	issues := config.Issues{}
	var body strings.Builder
	written := 0
	for i, s := range steps {
		if s.Enabled != nil && !*s.Enabled {
			continue
		}
		code, issue := e.write(base.Index(i), s)
		if issue != nil {
			issues.Add(*issue)
			continue
		}
		written++
		body.WriteString(fmt.Sprintf(string(TemplateStep), written, e.comment(s), code))
	}
	if written == 0 && len(issues) == 0 {
		issues.Add(config.NewError(config.IssueStepUnsupported, base, "%s", MsgNothing))
	}
	if len(issues) > 0 {
		return config.Step{}, issues.Sort()
	}
	code := fmt.Sprintf(string(TemplateHeader), written) + body.String() + string(TemplateFooter)
	return config.Step{Kind: config.StepKindJS, Code: &code}, nil
}

func (e *JsExporter) write(ptr config.Pointer, s config.Step) (string, *config.Issue) {
	w, ok := e.writers.Writer(s.Kind)
	if !ok {
		issue := config.NewError(config.IssueStepUnsupported, ptr.Token(config.TokenKind), string(MsgKindUnsupported), s.Kind)
		return "", &issue
	}
	if s.Strict != nil && *s.Strict {
		issue := config.NewError(config.IssueStepUnsupported, ptr, "%s", MsgStrict)
		return "", &issue
	}
	code, err := w.Write(s)
	if err != nil {
		issue := err.Issue(ptr)
		return "", &issue
	}
	return code, nil
}

func (e *JsExporter) comment(s config.Step) string {
	return strings.ReplaceAll(e.describer.Describe(s), string(LineBreak), string(LineSpace))
}
