// Package library holds the transform library: reusable transforms (versioned step lists) and lookup
// tables (versioned rows). Both kinds share one store and service, selected through a KindManager.
package library

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// ObjectKind names a kind of library object (also its audit object type and RBAC resource type).
type ObjectKind string

const (
	KindTransform   ObjectKind = "transform"
	KindLookupTable ObjectKind = "lookup_table"
)

// Item is one library object with its metadata, latest version summary and how many routes use it.
type Item struct {
	ID            uuid.UUID
	Kind          ObjectKind
	TeamID        uuid.UUID
	TeamName      string
	Name          string
	Description   string
	KeyType       ValueType
	ArchivedAt    *time.Time
	RowVersion    store.RowVersion
	LatestVersion int
	Size          int
	UsageCount    int
	UpdatedAt     time.Time
	UpdatedBy     string
}

// Version is one immutable version of an object: steps of a transform, rows of a lookup table.
type Version struct {
	No         int
	Content    json.RawMessage
	Size       int
	ConfigHash string
	ChangeNote string
	CreatedAt  time.Time
	CreatedBy  string
}

// Draft is a new object with its first version.
type Draft struct {
	TeamID      uuid.UUID
	Name        string
	Description string
	KeyType     ValueType
	Content     json.RawMessage
	ChangeNote  string
	Author      uuid.UUID
}

// VersionDraft is a new immutable version of an existing object.
type VersionDraft struct {
	Content    json.RawMessage
	ChangeNote string
	Author     uuid.UUID
}

// MetadataPatch changes an object's mutable metadata; nil members stay unchanged.
type MetadataPatch struct {
	Description *string
}

// Usage is one route referencing an object: the pinned versions and where (draft / active version).
type Usage struct {
	RouteID   uuid.UUID
	RouteName string
	Versions  []int
	InDraft   bool
	InActive  bool
}

// ListQuery filters a listing by name fragment or exact names, with keyset paging by name.
type ListQuery struct {
	Text            string
	Names           []string
	IncludeArchived bool
	Limit           int
	After           *store.Cursor
}

// ListPage is one page of objects; Next is nil on the last page.
type ListPage struct {
	Items []Item
	Next  *store.Cursor
}

// RowsQuery pages the rows of one lookup table version by position.
type RowsQuery struct {
	Version int
	Offset  int
	Limit   int
}

// RowsPage is one page of rows; Next is nil on the last page.
type RowsPage struct {
	Rows  []LookupRow
	Total int
	Next  *store.Cursor
}

// Limits caps versions: steps per transform, rows and bytes per lookup table version.
type Limits struct {
	MaxSteps          int
	MaxLookupRows     int
	MaxLookupBytes    int
	MaxListItems      int
	MaxDescription    int
	MaxChangeNote     int
	MaxImportIssues   int
	MaxNameCandidates int
}

// DefaultLimits follow design §2.1 (50k rows / 5 MB per lookup version).
var DefaultLimits = Limits{MaxSteps: 100, MaxLookupRows: 50_000, MaxLookupBytes: 5 << 20, MaxListItems: 200,
	MaxDescription: 2000, MaxChangeNote: 500, MaxImportIssues: 100, MaxNameCandidates: 100}

// Field names the JSON Pointers of request fields in validation errors.
type Field string

const (
	FieldName        Field = "/name"
	FieldSteps       Field = "/steps"
	FieldRows        Field = "/rows"
	FieldOwner       Field = "/teamId"
	FieldDescription Field = "/description"
	FieldChangeNote  Field = "/changeNote"
	FieldKeyType     Field = "/keyType"
	FieldLines       Field = "/lines"
	FieldContent     Field = ""
)

// Code names validation error codes.
type Code string

const (
	CodeRequired Code = "required"
	CodeInvalid  Code = "invalid"
	CodeTooLarge Code = "too_large"
	CodeDenied   Code = "forbidden"
	CodeCompile  Code = "compile"
	CodeInUse    Code = "in_use"
	CodeArchived Code = "archived"
	CodeTaken    Code = "name_taken"
)

// Message holds validation texts shown to users.
type Message string

const (
	MsgNameRequired   Message = "Give the object a name."
	MsgNamePattern    Message = "Use lower-case letters, digits and dashes (e.g. mask-customer-pii)."
	MsgNameTaken      Message = "A library object with this name already exists. Publish a new version of it or pick another name."
	MsgStepsRequired  Message = "Add at least one step."
	MsgTooManySteps   Message = "Too many steps for one library transform."
	MsgRowsRequired   Message = "Add at least one row."
	MsgRowsInvalid    Message = "Rows must be a list of {\"key\": \"…\", \"value\": …} objects."
	MsgRowKey         Message = "Every row needs a non-empty, unique key."
	MsgRowKeyType     Message = "The key does not match the table's key type (%s)."
	MsgTooManyRows    Message = "Too many rows for one lookup table version (max 50,000)."
	MsgTooLarge       Message = "The lookup table version is too large (max 5 MB)."
	MsgTeamNotMember  Message = "You cannot write to this team's library. Pick one of your teams."
	MsgTeamRequired   Message = "You are not an editor of any team yet. Ask an admin to add you to a team."
	MsgDescription    Message = "Use a shorter description."
	MsgChangeNote     Message = "Use a shorter change note."
	MsgKeyType        Message = "Use one of string, number or boolean."
	MsgArchived       Message = "This object is archived. Archived objects get no new versions."
	MsgInUse          Message = "Used by %d route(s): %s. Remove the references before archiving."
	MsgCompile        Message = "The steps do not compile: %s"
	MsgVersionMissing Message = "This version does not exist."
)

// ErrUnknownKind is returned when no ContentKind is registered for a kind.
var ErrUnknownKind = errors.New("library: unknown object kind")

// ErrNotFound is returned for a missing object.
var ErrNotFound = store.ErrNotFound

// ErrVersionRace reports a concurrent publish of the same object.
var ErrVersionRace = &store.Error{Kind: problem.KindConflict, Detail: "Someone published a version at the same time. Reload and try again."}

// Member names JSON members addressed in validation paths.
type Member string

const MemberRowKey Member = "key"
