package assist

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/path"
)

// SchemaFragment names the step definition inside the embedded route schema.
type SchemaFragment string

const StepFragment SchemaFragment = "#/$defs/step"

// PathMessage is the issue text of a malformed field path.
type PathMessage string

const MsgPathInvalid PathMessage = "This field path is not valid: %v"

// StepChecker validates single steps against the route schema's step definition and the path grammar.
type StepChecker struct {
	schema  *jsonschema.Schema
	printer *message.Printer
}

// NewStepChecker compiles the embedded step schema once.
func NewStepChecker() (*StepChecker, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(config.SchemaCatalog{}.RouteV1()))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(string(config.SchemaIDRouteV1), doc); err != nil {
		return nil, err
	}
	sch, err := c.Compile(string(config.SchemaIDRouteV1) + string(StepFragment))
	if err != nil {
		return nil, err
	}
	return &StepChecker{schema: sch, printer: message.NewPrinter(language.English)}, nil
}

// Check returns the issues of step s located under base (e.g. /steps/2).
func (c *StepChecker) Check(base config.Pointer, s config.Step) config.Issues {
	issues := c.schemaIssues(base, s)
	if len(issues) > 0 {
		return issues
	}
	return StepPaths{}.Issues(base, s)
}

func (c *StepChecker) schemaIssues(base config.Pointer, s config.Step) config.Issues {
	raw, err := json.Marshal(s)
	if err != nil {
		return config.Issues{config.NewError(config.IssueSchemaInvalid, base, "%v", err)}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return config.Issues{config.NewError(config.IssueSchemaInvalid, base, "%v", err)}
	}
	var ve *jsonschema.ValidationError
	if verr := c.schema.Validate(inst); !errors.As(verr, &ve) {
		return nil
	}
	issues := config.Issues{}
	for _, leaf := range c.leaves(ve, nil) {
		ptr := base.Join(config.PointerFromTokens(leaf.InstanceLocation))
		issues.Add(config.NewError(config.IssueSchemaInvalid, ptr, "%s", leaf.ErrorKind.LocalizedString(c.printer)))
	}
	return issues
}

func (c *StepChecker) leaves(e *jsonschema.ValidationError, acc []*jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return append(acc, e)
	}
	for _, cause := range e.Causes {
		acc = c.leaves(cause, acc)
	}
	return acc
}

// StepPaths checks every field path member of a step with the engine's path grammar.
type StepPaths struct{}

// Issues returns one path_invalid issue per malformed path.
func (StepPaths) Issues(base config.Pointer, s config.Step) config.Issues {
	issues := config.Issues{}
	single := map[config.PointerToken]*string{config.TokenFrom: s.From, config.TokenTo: s.To, config.TokenField: s.Field}
	for tok, p := range single {
		if p == nil {
			continue
		}
		if _, err := (path.Parser{}).Parse(*p); err != nil {
			issues.Add(config.NewError(config.IssuePathInvalid, base.Token(tok), string(MsgPathInvalid), err))
		}
	}
	for i, p := range s.Fields {
		if _, err := (path.Parser{}).Parse(p); err != nil {
			issues.Add(config.NewError(config.IssuePathInvalid, base.Token(config.TokenFields).Index(i), string(MsgPathInvalid), err))
		}
	}
	return issues.Sort()
}

// StepCodec decodes and encodes step arrays exchanged with providers and clients.
type StepCodec struct{}

// Decode parses a JSON array of steps.
func (StepCodec) Decode(raw json.RawMessage) ([]config.Step, error) {
	if len(raw) == 0 {
		return []config.Step{}, nil
	}
	var steps []config.Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// Encode renders steps as a JSON array.
func (StepCodec) Encode(steps []config.Step) (json.RawMessage, error) {
	if steps == nil {
		steps = []config.Step{}
	}
	return json.Marshal(steps)
}
