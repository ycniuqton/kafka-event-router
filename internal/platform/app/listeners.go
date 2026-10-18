package app

import (
	"context"
	"errors"
	"log/slog"
)

// Runnable is a listener served until ctx ends.
type Runnable interface {
	Run(ctx context.Context) error
}

// Listeners runs the public and internal servers with a shared shutdown: when one stops, all stop.
type Listeners struct {
	log     *slog.Logger
	servers []Runnable
}

// Add registers a server.
func (l *Listeners) Add(s Runnable) {
	l.servers = append(l.servers, s)
}

// Run serves all listeners and returns their joined errors after every one has stopped.
func (l *Listeners) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(l.servers))
	for _, s := range l.servers {
		go func(s Runnable) { results <- s.Run(ctx) }(s)
	}
	var errs []error
	for range l.servers {
		errs = append(errs, <-results)
		cancel()
	}
	return errors.Join(errs...)
}
