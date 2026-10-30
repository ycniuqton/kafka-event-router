package engine

import (
	"errors"
	"fmt"
)

// DependencyKind names an external dependency a step or codec calls while processing a record.
type DependencyKind string

const (
	DependencyLookup         DependencyKind = "lookup"
	DependencySchemaRegistry DependencyKind = "schema_registry"
	DependencyHTTP           DependencyKind = "http"
)

// DependencyError is the transient-error contract: the dependency could not answer (unreachable, timeout,
// 5xx, pool exhausted) and the record itself is fine. Wrap it as the Cause of a ProcessError; a lookup
// miss or a bad key is not a DependencyError (that is bad data).
type DependencyError struct {
	Kind  DependencyKind
	Name  string
	Cause error
}

func (e *DependencyError) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("%s %q unavailable: %v", e.Kind, e.Name, e.Cause)
	}
	return fmt.Sprintf("%s unavailable: %v", e.Kind, e.Cause)
}

func (e *DependencyError) Unwrap() error { return e.Cause }

// Transient marks the error as healing on retry (runtime TransientCauses and DLQ classification read it).
func (e *DependencyError) Transient() bool { return true }

// AsDependencyError returns the DependencyError in err's chain.
func AsDependencyError(err error) (*DependencyError, bool) {
	var de *DependencyError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}
