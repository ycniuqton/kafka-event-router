// Package diff renders readable, section-by-section differences between two route configs
// (source, filter rows, transform steps, destinations, settings) for History and Review.
package diff

import (
	"github.com/eventrouter/router/pkg/config"
)

// SectionKind names a diff section.
type SectionKind string

const (
	SectionSource      SectionKind = "source"
	SectionFilter      SectionKind = "filter"
	SectionTransform   SectionKind = "transform"
	SectionDestination SectionKind = "destination"
	SectionSettings    SectionKind = "settings"
)

// Mark is the kind of change of one line.
type Mark string

const (
	MarkAdded   Mark = "+"
	MarkRemoved Mark = "-"
	MarkChanged Mark = "~"
)

// Line is one readable change.
type Line struct {
	Mark Mark   `json:"mark"`
	Text string `json:"text"`
	Path string `json:"path"`
}

// SectionDiff holds the lines of one section (empty when nothing changed).
type SectionDiff struct {
	Section SectionKind `json:"section"`
	Lines   []Line      `json:"lines"`
}

// RouteDiff is the response of GET /routes/{id}/versions/{a}/diff/{b}.
type RouteDiff struct {
	A        string        `json:"a"`
	B        string        `json:"b"`
	Sections []SectionDiff `json:"sections"`
}

// Changed reports whether any section has lines.
func (d RouteDiff) Changed() bool {
	for _, s := range d.Sections {
		if len(s.Lines) > 0 {
			return true
		}
	}
	return false
}

// Section extracts comparable entries of one aspect of a config.
type Section interface {
	Section() SectionKind
	Entries(cfg *config.RouteConfig) []Entry
}

// RouteDiffer runs every registered section; a nil config is an empty side (all lines "+" or "-").
type RouteDiffer struct {
	sections []Section
}

// NewRouteDiffer registers the sections in display order.
func NewRouteDiffer(sections ...Section) *RouteDiffer {
	return &RouteDiffer{sections: sections}
}

// DefaultRouteDiffer has source, filter, transform, destination and settings.
func DefaultRouteDiffer() *RouteDiffer {
	return NewRouteDiffer(SourceSection{}, FilterSection{}, TransformSection{}, DestinationSection{}, SettingsSection{})
}

// Diff compares a and b section by section.
func (d *RouteDiffer) Diff(aRef, bRef string, a, b *config.RouteConfig) RouteDiff {
	out := RouteDiff{A: aRef, B: bRef, Sections: make([]SectionDiff, 0, len(d.sections))}
	for _, s := range d.sections {
		out.Sections = append(out.Sections, SectionDiff{Section: s.Section(), Lines: EntryComparer{}.Compare(d.entries(s, a), d.entries(s, b))})
	}
	return out
}

func (*RouteDiffer) entries(s Section, cfg *config.RouteConfig) []Entry {
	if cfg == nil {
		return nil
	}
	return s.Entries(cfg)
}
