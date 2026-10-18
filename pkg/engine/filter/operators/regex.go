package operators

import (
	"regexp"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Regex matches strings against an RE2 pattern compiled once.
type Regex struct{ base }

func (Regex) Op() config.OpName           { return config.OpNameRegex }
func (Regex) Accepts() []filter.FieldType { return stringTypes }
func (Regex) Operand() filter.OperandKind { return filter.OperandValue }
func (Regex) Prepare(o filter.Operand) (filter.Prepared, error) {
	s, ok := o.Value.(string)
	if !ok {
		return nil, filter.NewOperandError(config.IssueRegexInvalid, config.TokenValue, "regex requires a string pattern")
	}
	re, err := regexp.Compile(s)
	if err != nil {
		return nil, filter.NewOperandError(config.IssueRegexInvalid, config.TokenValue, "%v", err)
	}
	return regexMatch{re: re}, nil
}

type regexMatch struct {
	re *regexp.Regexp
}

func (p regexMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	s, ok := in.Left.(string)
	if !ok {
		return false, &filter.CoercionWarning{Code: filter.WarnIncomparable}
	}
	return p.re.MatchString(s), nil
}
