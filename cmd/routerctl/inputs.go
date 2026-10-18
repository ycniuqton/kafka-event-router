package main

import (
	"io"
	"os"

	"github.com/eventrouter/router/pkg/config"
)

// StdinPath selects standard input as the events file.
const StdinPath = "-"

// InputFile opens the events file or stdin.
type InputFile struct {
	Path string
}

// Open returns the reader and its closer.
func (f InputFile) Open() (io.Reader, func(), error) {
	if f.Path == StdinPath {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

// SourceTopic picks the topic stamped on file events.
type SourceTopic struct{}

// Of returns the first source topic, or "" when the route uses a pattern.
func (SourceTopic) Of(cfg *config.RouteConfig) string {
	if cfg == nil || len(cfg.Source.Topics) == 0 {
		return ""
	}
	return cfg.Source.Topics[0]
}
