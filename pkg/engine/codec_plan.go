package engine

import (
	"context"
	"fmt"
)

// CodecMode selects how JSON record values are decoded and encoded on the record path.
type CodecMode string

const (
	// CodecFast decodes only the fields a route touches and patches the original bytes.
	CodecFast CodecMode = "fast"
	// CodecTree decodes the whole value into a tree and re-encodes it.
	CodecTree CodecMode = "tree"
)

// DefaultCodecMode applies when no mode is configured.
const DefaultCodecMode = CodecFast

// CodecModes parses configured codec modes.
type CodecModes struct{}

// Parse returns the mode named by s; empty selects DefaultCodecMode.
func (CodecModes) Parse(s string) (CodecMode, error) {
	switch m := CodecMode(s); m {
	case "":
		return DefaultCodecMode, nil
	case CodecFast, CodecTree:
		return m, nil
	}
	return "", fmt.Errorf("unknown engine codec %q (want %s or %s)", s, CodecFast, CodecTree)
}

// OrDefault returns m, or DefaultCodecMode when m is empty.
func (m CodecMode) OrDefault() CodecMode {
	if m == "" {
		return DefaultCodecMode
	}
	return m
}

// CodecFallback is why a route runs the tree codec although the fast one was requested.
type CodecFallback string

const (
	FallbackNone              CodecFallback = ""
	FallbackModeTree          CodecFallback = "mode_tree"
	FallbackSourceFormat      CodecFallback = "source_format"
	FallbackDestinationFormat CodecFallback = "destination_format"
	FallbackScript            CodecFallback = "script"
	FallbackStepKind          CodecFallback = "step_kind"
	FallbackFilterNode        CodecFallback = "filter_node"
	FallbackValueSource       CodecFallback = "value_source"
	FallbackFormula           CodecFallback = "formula"
	FallbackWholeValue        CodecFallback = "whole_value"
	FallbackUnanalysed        CodecFallback = "unanalysed"
)

// CodecPlan is the codec a compiled route runs with and, for the tree codec, why.
type CodecPlan struct {
	Mode     CodecMode
	Fallback CodecFallback
	Detail   string
}

// Fast reports whether the route runs the fast codec.
func (p CodecPlan) Fast() bool { return p.Mode == CodecFast }

// String renders the plan for logs.
func (p CodecPlan) String() string {
	if p.Fallback == FallbackNone {
		return string(p.Mode)
	}
	if p.Detail == "" {
		return fmt.Sprintf("%s (%s)", p.Mode, p.Fallback)
	}
	return fmt.Sprintf("%s (%s: %s)", p.Mode, p.Fallback, p.Detail)
}

// CodecStats counts records of a fast-codec route that had to re-run on the tree codec
// (not an object, invalid JSON, or any decode / process / encode error).
type CodecStats struct {
	Fallbacks uint64
}

// CodecStatsReporter is implemented by pipelines that count codec fallbacks.
type CodecStatsReporter interface {
	CodecStats() CodecStats
}

// Encoded is one destination record with the branch that produced it.
type Encoded struct {
	Record RawRecord
	Branch BranchID
}

// RecordRunner is implemented by pipelines that run decode → process → encode in one call,
// so the codec can keep a private value representation; results and errors are the same
// as Decode, Process (no tracer) and Encode of every output in order.
type RecordRunner interface {
	Run(ctx context.Context, r RawRecord) ([]Encoded, error)
	CodecPlan() CodecPlan
}
