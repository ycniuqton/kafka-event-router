package validation

import "context"

// ConnectionInfo is what validators need to know about a connection.
type ConnectionInfo struct {
	ID                string
	TeamID            string
	Name              string
	EnvTag            string
	HasSchemaRegistry bool
}

// ConnectionLookup resolves connection ids (connections; Postgres adapter in routestore).
type ConnectionLookup interface {
	Find(ctx context.Context, ids []string) (map[string]ConnectionInfo, error)
}

// TopicInspector checks topics on a cluster (connections' pkg/kafka Admin).
type TopicInspector interface {
	TopicExists(ctx context.Context, connectionID, topic string) (bool, error)
}

// FieldType is the observed type of a sample field.
type FieldType string

const (
	FieldString    FieldType = "string"
	FieldNumber    FieldType = "number"
	FieldBoolean   FieldType = "boolean"
	FieldObject    FieldType = "object"
	FieldArray     FieldType = "array"
	FieldNull      FieldType = "null"
	FieldTimestamp FieldType = "timestamp"
)

// FieldSchema maps field paths ("value.amount") to their observed types.
type FieldSchema map[string]FieldType

// SchemaRequest names the source whose latest samples define the schema.
type SchemaRequest struct {
	TeamID       string
	ConnectionID string
	Topics       []string
}

// SampleSchemaProvider returns the latest sample schema; ok=false when no samples exist (live-preview).
type SampleSchemaProvider interface {
	LatestSchema(ctx context.Context, req SchemaRequest) (schema FieldSchema, ok bool, err error)
}

// TestCaseResult is one saved test case outcome (transform-studio, P2).
type TestCaseResult struct {
	Name    string
	Passed  bool
	Message string
}

// TestCaseRunner runs a route's saved test cases against a document (P2).
type TestCaseRunner interface {
	Run(ctx context.Context, routeID string, raw []byte) ([]TestCaseResult, error)
}

// LibraryKind names the kind of library object a step references.
type LibraryKind string

const (
	LibraryTransform   LibraryKind = "transform"
	LibraryLookupTable LibraryKind = "lookup_table"
)

// LibraryRef names one library object of a team.
type LibraryRef struct {
	TeamID string
	Kind   LibraryKind
	Name   string
}

// LibraryLookup resolves library refs to their newest version; ok=false for unknown names (transform-studio, P3).
type LibraryLookup interface {
	Latest(ctx context.Context, ref LibraryRef) (version int, ok bool, err error)
}

// GrantChecker decides whether a team may use another team's connection (access-control, P2).
type GrantChecker interface {
	CanUseConnection(ctx context.Context, teamID, connectionID string) (bool, error)
}

// RouteNameIndex answers whether a name is used by another route of the team.
type RouteNameIndex interface {
	NameTakenByOther(ctx context.Context, teamID, name, exceptRouteID string) (bool, error)
}
