package quickjs

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/eventrouter/router/pkg/sandbox"
)

// scriptName is a file name the guest reports in stack traces.
type scriptName string

const (
	userScript     scriptName = "step.js"
	resolverScript scriptName = "sandbox-entry.js"
)

// guestEntry describes how one EntryKind is wrapped and resolved in the guest.
type guestEntry struct {
	function   string
	prefix     string
	suffix     string
	lineOffset int
}

var guestEntries = map[sandbox.EntryKind]guestEntry{
	sandbox.EntryTransform: {function: "transform", suffix: "\n"},
	sandbox.EntryPredicate: {
		function:   "__sb_predicate",
		prefix:     "function __sb_predicate(event, ctx) {\nreturn (\n",
		suffix:     "\n);\n}\n",
		lineOffset: 2,
	},
}

// guestSource is user code prepared for the guest.
type guestSource struct {
	entry     guestEntry
	code      string
	userLines []string
}

// newGuestSource wraps src per its entry kind; empty predicates are rejected.
func newGuestSource(src sandbox.Source) (guestSource, *sandbox.Error) {
	entry, ok := guestEntries[src.Entry]
	if !ok {
		return guestSource{}, &sandbox.Error{Kind: sandbox.ErrorCompile, Message: sandbox.MsgUnsupportedEntry.Format(src.Entry)}
	}
	if src.Entry == sandbox.EntryPredicate && strings.TrimSpace(src.Code) == "" {
		return guestSource{}, &sandbox.Error{Kind: sandbox.ErrorCompile, Message: string(sandbox.MsgEmptyPredicate)}
	}
	return guestSource{entry: entry, code: entry.prefix + src.Code + entry.suffix, userLines: strings.Split(src.Code, "\n")}, nil
}

// position returns the mapper from guest stack positions to user-code positions.
func (g guestSource) position() stackPosition {
	return stackPosition{lineOffset: g.entry.lineOffset, userLines: g.userLines}
}

// resolver is the script whose completion value is the entry function (or undefined).
func (g guestSource) resolver() string {
	return "(typeof " + g.entry.function + " === 'function' ? " + g.entry.function + " : undefined)"
}

// stackPosition extracts the user-code line and column from a QuickJS stack trace.
type stackPosition struct {
	lineOffset int
	userLines  []string
}

var userFramePattern = regexp.MustCompile(regexp.QuoteMeta(string(userScript)) + `:(\d+):(\d+)`)

// find returns the first user-code position, shifted back by the wrapper lines.
func (p stackPosition) find(stack string) (line, column int) {
	m := userFramePattern.FindStringSubmatch(stack)
	if m == nil {
		return 0, 0
	}
	line, _ = strconv.Atoi(m[1])
	column, _ = strconv.Atoi(m[2])
	return p.clamp(line-p.lineOffset, column)
}

// clamp maps positions inside the wrapper's trailer to the end of the user's last line.
func (p stackPosition) clamp(line, column int) (int, int) {
	last := len(p.userLines)
	if last == 0 || (line >= 1 && line <= last) {
		return max(line, 1), column
	}
	if line < 1 {
		return 1, 1
	}
	return last, len(p.userLines[last-1]) + 1
}
