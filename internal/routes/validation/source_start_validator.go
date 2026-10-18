package validation

import (
	"context"
	"regexp"
	"strconv"

	"github.com/eventrouter/router/pkg/config"
)

// Issue codes of the source shape rules.
const (
	IssueTopicsAndPattern config.IssueCode = "topics_and_pattern"
	IssueNoTopics         config.IssueCode = "no_topics"
	IssueOffsetsInvalid   config.IssueCode = "offsets_invalid"
	IssueStartAtMissing   config.IssueCode = "start_at_missing"
)

// Source members addressed by the shape rules.
const (
	MemberTopics       DocumentMember = "topics"
	MemberTopicPattern DocumentMember = "topicPattern"
	MemberStartFrom    DocumentMember = "startFrom"
	MemberOffsets      DocumentMember = "offsets"
	MemberAt           DocumentMember = "at"
)

// noTopicsSeverity: a draft may still lack topics; deploying one cannot.
var noTopicsSeverity = map[ValidationMode]config.Severity{
	ModeDraft: config.SeverityWarning, ModeImport: config.SeverityWarning, ModeDeploy: config.SeverityError, ModeRollback: config.SeverityError,
}

// SourcePointer addresses the source object.
var SourcePointer = config.PointerRoot.Token(config.TokenSource)

// SourceStartValidator checks the P2 source modes the schema cannot express: topics xor a (Go RE2) topic
// pattern, offsets keyed by partition number for mode=offsets, and an instant for mode=timestamp.
type SourceStartValidator struct{}

func (SourceStartValidator) Name() ValidatorName     { return ValidatorSourceStart }
func (SourceStartValidator) Modes() []ValidationMode { return AllModes }

// Validate returns errors on the offending members.
func (v SourceStartValidator) Validate(_ context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	src := in.Config.Source
	issues := v.topics(src, in.Mode)
	if src.StartFrom != nil {
		issues = append(issues, v.start(*src.StartFrom)...)
	}
	return issues, nil
}

// topics: exactly one of topics / topicPattern, and the pattern must compile.
func (SourceStartValidator) topics(src config.Source, mode ValidationMode) []config.Issue {
	pattern := ""
	if src.TopicPattern != nil {
		pattern = *src.TopicPattern
	}
	at := SourcePointer.Key(string(MemberTopicPattern))
	switch {
	case pattern != "" && len(src.Topics) > 0:
		return []config.Issue{config.NewError(IssueTopicsAndPattern, at, "Use either a topic list or a topic pattern, not both.")}
	case pattern == "" && len(src.Topics) == 0:
		is := config.NewError(IssueNoTopics, SourcePointer.Key(string(MemberTopics)), "Pick at least one topic or enter a topic pattern.")
		is.Severity = noTopicsSeverity[mode]
		return []config.Issue{is}
	case pattern == "":
		return nil
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return []config.Issue{config.NewError(config.IssueRegexInvalid, at, "The topic pattern is not a valid regular expression: %v", err)}
	}
	return nil
}

// start: offsets need partition keys, timestamp needs an instant.
func (SourceStartValidator) start(s config.StartFrom) []config.Issue {
	base := SourcePointer.Key(string(MemberStartFrom))
	if s.Mode == config.StartModeTimestamp && s.At == nil {
		return []config.Issue{config.NewError(IssueStartAtMissing, base.Key(string(MemberAt)), "Pick the date and time to start from.")}
	}
	if s.Mode != config.StartModeOffsets {
		return nil
	}
	if len(s.Offsets) == 0 {
		return []config.Issue{config.NewError(IssueOffsetsInvalid, base.Key(string(MemberOffsets)), "Enter a start offset for at least one partition.")}
	}
	issues := []config.Issue{}
	for partition := range s.Offsets {
		if n, err := strconv.Atoi(partition); err != nil || n < 0 {
			issues = append(issues, config.NewError(IssueOffsetsInvalid, base.Key(string(MemberOffsets)).Key(partition),
				"Offsets are keyed by partition number (0, 1, 2 …); %q is not one.", partition))
		}
	}
	return issues
}
