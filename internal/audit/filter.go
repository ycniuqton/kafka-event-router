package audit

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Filter narrows an audit query; nil/zero members do not filter.
type Filter struct {
	Actor         *uuid.UUID
	Actions       []Action
	ObjectType    *ObjectType
	ObjectID      *string
	TeamID        *uuid.UUID
	From, To      *time.Time
	SensitiveOnly bool
	Text          string
}

// FilterParam names the query parameters of the audit log endpoints.
type FilterParam string

const (
	ParamActor      FilterParam = "actor"
	ParamAction     FilterParam = "action"
	ParamObjectType FilterParam = "objectType"
	ParamObjectID   FilterParam = "objectId"
	ParamTeamID     FilterParam = "teamId"
	ParamFrom       FilterParam = "from"
	ParamTo         FilterParam = "to"
	ParamSensitive  FilterParam = "sensitive"
	ParamText       FilterParam = "q"
	ParamFormat     FilterParam = "format"
)

// FilterRule names the validation codes and limits of filter parsing.
type FilterRule string

const (
	RuleUUID      FilterRule = "uuid"
	RuleAction    FilterRule = "unknown_action"
	RuleTime      FilterRule = "datetime"
	RuleRange     FilterRule = "range"
	RuleBoolean   FilterRule = "boolean"
	RuleMaxLength FilterRule = "max_length"
	RuleFormat    FilterRule = "format"
)

// FilterLimit bounds free-text search.
type FilterLimit int

const MaxTextLength FilterLimit = 200

// ListSeparator splits multi-valued parameters.
type ListSeparator string

const ActionSeparator ListSeparator = ","

// FilterError is the 422 returned for invalid filter parameters (one FieldError per parameter).
type FilterError struct {
	Errors []problem.FieldError
}

func (e *FilterError) Error() string {
	return "Some audit filters are not valid. Fix them and try again."
}
func (e *FilterError) ProblemKind() problem.Kind         { return problem.KindValidation }
func (e *FilterError) FieldErrors() []problem.FieldError { return e.Errors }

// FilterParser reads a Filter from query parameters and validates actions against the catalog.
type FilterParser struct {
	catalog *Catalog
}

// NewFilterParser builds a parser over the catalog.
func NewFilterParser(c *Catalog) *FilterParser {
	return &FilterParser{catalog: c}
}

// Parse validates every parameter and returns all problems at once.
func (p *FilterParser) Parse(q url.Values) (Filter, error) {
	r := &filterReader{q: q, errs: &FilterError{}}
	f := Filter{
		Actor:         r.uuid(ParamActor),
		Actions:       r.actions(p.catalog),
		ObjectType:    r.objectType(),
		ObjectID:      r.optional(ParamObjectID),
		TeamID:        r.uuid(ParamTeamID),
		From:          r.time(ParamFrom),
		To:            r.time(ParamTo),
		SensitiveOnly: r.boolean(ParamSensitive),
		Text:          r.text(),
	}
	r.checkRange(f)
	if len(r.errs.Errors) > 0 {
		return Filter{}, r.errs
	}
	return f, nil
}

// filterReader accumulates per-parameter parse errors.
type filterReader struct {
	q    url.Values
	errs *FilterError
}

func (r *filterReader) fail(param FilterParam, rule FilterRule, message string) {
	r.errs.Errors = append(r.errs.Errors, problem.FieldError{Path: "/" + string(param), Code: string(rule), Message: message})
}

func (r *filterReader) optional(param FilterParam) *string {
	v := strings.TrimSpace(r.q.Get(string(param)))
	if v == "" {
		return nil
	}
	return &v
}

func (r *filterReader) uuid(param FilterParam) *uuid.UUID {
	raw := r.optional(param)
	if raw == nil {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		r.fail(param, RuleUUID, "Use the id of an existing object.")
		return nil
	}
	return &id
}

func (r *filterReader) actions(c *Catalog) []Action {
	raw := r.optional(ParamAction)
	if raw == nil {
		return nil
	}
	var out []Action
	for _, part := range strings.Split(*raw, string(ActionSeparator)) {
		a := Action(strings.TrimSpace(part))
		if _, ok := c.Describe(a); !ok {
			r.fail(ParamAction, RuleAction, "Unknown action "+string(a)+". Pick one from the action list.")
			continue
		}
		out = append(out, a)
	}
	return out
}

func (r *filterReader) objectType() *ObjectType {
	raw := r.optional(ParamObjectType)
	if raw == nil {
		return nil
	}
	t := ObjectType(*raw)
	return &t
}

func (r *filterReader) time(param FilterParam) *time.Time {
	raw := r.optional(param)
	if raw == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		r.fail(param, RuleTime, "Use an RFC 3339 time such as 2026-10-13T09:00:00Z.")
		return nil
	}
	return &t
}

func (r *filterReader) boolean(param FilterParam) bool {
	raw := r.optional(param)
	if raw == nil {
		return false
	}
	b, err := strconv.ParseBool(*raw)
	if err != nil {
		r.fail(param, RuleBoolean, "Use true or false.")
	}
	return b
}

func (r *filterReader) text() string {
	raw := r.optional(ParamText)
	if raw == nil {
		return ""
	}
	if len(*raw) > int(MaxTextLength) {
		r.fail(ParamText, RuleMaxLength, "Search text is too long. Use at most 200 characters.")
		return ""
	}
	return *raw
}

func (r *filterReader) checkRange(f Filter) {
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		r.fail(ParamTo, RuleRange, "The end of the time range must be after its start.")
	}
}
