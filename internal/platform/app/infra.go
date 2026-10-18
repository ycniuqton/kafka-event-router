package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/store"
)

// Infra names a shared infrastructure dependency a service can request.
type Infra string

const (
	InfraPostgres Infra = "postgres"
	InfraRedis    Infra = "redis"
)

// InfraOpener opens requested infrastructure and registers readiness checks and closers.
type InfraOpener struct{}

// openers maps each infra kind to its opening step.
var openers = map[Infra]func(ctx context.Context, deps *Deps, closer *Closer) error{
	InfraPostgres: InfraOpener{}.postgres,
	InfraRedis:    InfraOpener{}.redis,
}

// Open opens every requested dependency.
func (o InfraOpener) Open(ctx context.Context, infra []Infra, deps *Deps, closer *Closer) error {
	for _, kind := range infra {
		open, ok := openers[kind]
		if !ok {
			return fmt.Errorf("app: unknown infra %q", kind)
		}
		if err := open(ctx, deps, closer); err != nil {
			return err
		}
	}
	return nil
}

func (InfraOpener) postgres(ctx context.Context, deps *Deps, closer *Closer) error {
	db, err := store.Open(ctx, deps.Config.Database)
	if err != nil {
		return err
	}
	deps.DB = db
	deps.Health.AddCheck(db)
	closer.Add(func() error { db.Close(); return nil })
	return nil
}

func (InfraOpener) redis(_ context.Context, deps *Deps, closer *Closer) error {
	opts, err := redis.ParseURL(deps.Config.Redis.URL)
	if err != nil {
		return fmt.Errorf("app: redis url: %w", err)
	}
	client := redis.NewClient(opts)
	deps.Redis = client
	deps.Health.AddCheck(RedisCheck{client: client})
	closer.Add(client.Close)
	return nil
}

// RedisCheck pings Redis for readiness.
type RedisCheck struct {
	client redis.UniversalClient
}

// Name implements httpserver.ReadinessCheck.
func (c RedisCheck) Name() string { return string(InfraRedis) }

// Check implements httpserver.ReadinessCheck.
func (c RedisCheck) Check(ctx context.Context) error { return c.client.Ping(ctx).Err() }

// Closer releases opened resources in reverse order.
type Closer struct {
	fns []func() error
}

// Add registers a release function.
func (c *Closer) Add(fn func() error) {
	c.fns = append(c.fns, fn)
}

// Close runs all release functions in reverse order.
func (c *Closer) Close() error {
	var errs []error
	for i := len(c.fns) - 1; i >= 0; i-- {
		errs = append(errs, c.fns[i]())
	}
	return errors.Join(errs...)
}
