package routes

import (
	"regexp"
	"strconv"
	"strings"
)

// NamingToken names the fixed parts of generated names.
type NamingToken string

const (
	CopySuffix          NamingToken = "-copy"
	CopyCounterSep      NamingToken = "-"
	ConsumerGroupPrefix NamingToken = "router."
	SlugReplacement     NamingToken = "-"
)

// FirstCopyCounter is the first numbered duplicate ("<name>-copy-2").
type FirstCopyCounter int

const firstCopyCounter FirstCopyCounter = 2

var (
	copySuffixPattern = regexp.MustCompile(`-copy(-\d+)?$`)
	slugInvalidChars  = regexp.MustCompile(`[^a-z0-9._-]+`)
)

// DuplicateNamer picks "<name>-copy", "<name>-copy-2" … not yet used in the team.
type DuplicateNamer struct{}

// Next returns the first free duplicate name for name given the team's taken names.
func (DuplicateNamer) Next(name string, taken map[string]bool) string {
	base := copySuffixPattern.ReplaceAllString(name, "") + string(CopySuffix)
	if !taken[base] {
		return base
	}
	for n := int(firstCopyCounter); ; n++ {
		candidate := base + string(CopyCounterSep) + strconv.Itoa(n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// Prefix returns the common prefix of every candidate, used to query taken names.
func (DuplicateNamer) Prefix(name string) string {
	return copySuffixPattern.ReplaceAllString(name, "") + string(CopySuffix)
}

// ConsumerGroupNamer derives the default consumer group "router.<route-name>".
type ConsumerGroupNamer struct{}

// For returns the default consumer group of a route name.
func (ConsumerGroupNamer) For(routeName string) string {
	slug := slugInvalidChars.ReplaceAllString(strings.ToLower(strings.TrimSpace(routeName)), string(SlugReplacement))
	return string(ConsumerGroupPrefix) + strings.Trim(slug, string(SlugReplacement))
}
