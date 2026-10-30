package lookup

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// Client calls one source; implementations exist per SourceKind and are built by a ClientFactory.
type Client interface {
	Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error)
	// Probe checks the source for the Test button; a failure is a *ProbeError naming its stage.
	Probe(ctx context.Context) error
	Close()
}

// ClientFactory builds clients of one kind.
type ClientFactory interface {
	Kind() SourceKind
	New(spec SourceSpec) (Client, error)
}

// ErrUnknownKind is returned for a kind without a registered factory.
var ErrUnknownKind = errors.New("lookup source kind is not supported")

// FactoryRegistry selects client factories by kind (the Gateway and Tester talk only to it).
type FactoryRegistry struct {
	factories map[SourceKind]ClientFactory
}

// NewFactoryRegistry registers factories.
func NewFactoryRegistry(fs ...ClientFactory) *FactoryRegistry {
	r := &FactoryRegistry{factories: map[SourceKind]ClientFactory{}}
	for _, f := range fs {
		r.Register(f)
	}
	return r
}

// DefaultFactories registers HTTP, Redis, Postgres and static (rows from tables).
func DefaultFactories(tables TableLoader) *FactoryRegistry {
	return NewFactoryRegistry(HTTPFactory{}, RedisFactory{}, PostgresFactory{}, StaticFactory{Tables: tables})
}

// Register adds or replaces a factory.
func (r *FactoryRegistry) Register(f ClientFactory) { r.factories[f.Kind()] = f }

// New builds the client of spec.
func (r *FactoryRegistry) New(spec SourceSpec) (Client, error) {
	f, ok := r.factories[spec.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, spec.Kind)
	}
	return f.New(spec)
}

// ProbeStage is one check of the Test button checklist.
type ProbeStage string

const (
	StageReachable     ProbeStage = "reachable"
	StageAuthenticated ProbeStage = "authenticated"
	StageSample        ProbeStage = "sample"
)

// ProbeStages lists the checklist in order.
var ProbeStages = []ProbeStage{StageReachable, StageAuthenticated, StageSample}

// ProbeError is a failed probe stage with a plain-language hint.
type ProbeError struct {
	Stage ProbeStage
	Hint  string
	Cause error
}

func (e *ProbeError) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Cause) }

func (e *ProbeError) Unwrap() error { return e.Cause }

// ProbeHint is the plain-language next step shown for a failed probe stage.
type ProbeHint string

const (
	HintReachable   ProbeHint = "Check the address, port and network access from the router to the source."
	HintCredentials ProbeHint = "Check the credentials: replace the secret or the user name."
	HintServerError ProbeHint = "The source answered with a server error; check its health."
	HintReadOnly    ProbeHint = "The read-only test query failed; check the database, user grants and statement timeout."
	HintTable       ProbeHint = "Check that the library lookup table and the pinned version exist and have rows."
)

// FailureReason is the plain-language reason of a SourceError.
type FailureReason string

const (
	ReasonUnavailable  FailureReason = "source unavailable"
	ReasonRejected     FailureReason = "source rejected the request"
	ReasonCircuitOpen  FailureReason = "circuit open"
	ReasonRateLimited  FailureReason = "rate limited"
	ReasonKindMismatch FailureReason = "request kind does not match the source kind"
	ReasonUnknown      FailureReason = "unknown lookup source"
	ReasonForbidden    FailureReason = "lookup source belongs to another team"
	ReasonBadAnswer    FailureReason = "unreadable answer"
)

// Failures builds classified SourceErrors.
type Failures struct{}

// Transient wraps a cause that may heal on retry.
func (Failures) Transient(reason FailureReason, cause error) error {
	return &ctxhelpers.SourceError{Transient: true, Reason: string(reason), Cause: cause}
}

// Permanent wraps a cause that will not heal by retrying.
func (Failures) Permanent(reason FailureReason, cause error) error {
	return &ctxhelpers.SourceError{Reason: string(reason), Cause: cause}
}

// Network classifies transport errors: deadlines and network errors are transient, the rest permanent.
func (f Failures) Network(err error) error {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) {
		return f.Transient(ReasonUnavailable, err)
	}
	return f.Permanent(ReasonRejected, err)
}
