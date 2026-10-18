package routestore

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// Params converts domain values into query parameters.
type Params struct{}

// UUID parses an id; malformed ids can never exist, so they map to ErrNotFound.
func (Params) UUID(id string) (uuid.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, store.ErrNotFound
	}
	return u, nil
}

// OptionalUUID converts an optional user id.
func (Params) OptionalUUID(id *uuid.UUID) *uuid.UUID {
	return id
}

// JSON passes a document as text for an explicit ::jsonb cast; nil stays SQL NULL.
func (Params) JSON(raw json.RawMessage) *string {
	if raw == nil {
		return nil
	}
	s := string(raw)
	return &s
}

// Values converts scanned optional columns into domain values.
type Values struct{}

// UserID parses an optional uuid text.
func (Values) UserID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil
	}
	return &u
}

// VersionID converts an optional id text.
func (Values) VersionID(s *string) *routes.VersionID {
	if s == nil {
		return nil
	}
	v := routes.VersionID(*s)
	return &v
}

// Raw keeps NULL jsonb as a nil document.
func (Values) Raw(b []byte) json.RawMessage {
	if b == nil {
		return nil
	}
	return json.RawMessage(b)
}

// UserRef builds an author reference when the user still exists.
func (Values) UserRef(id, name *string) *routes.UserRef {
	if id == nil {
		return nil
	}
	ref := routes.UserRef{ID: *id}
	if name != nil {
		ref.Name = *name
	}
	return &ref
}
