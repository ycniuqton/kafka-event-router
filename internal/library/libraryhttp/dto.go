package libraryhttp

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/store"
)

// ContentMember names the request/response member carrying a version's content.
type ContentMember string

const (
	MemberSteps ContentMember = "steps"
	MemberRows  ContentMember = "rows"
)

// ObjectInput is the body of create (name, owner team, metadata, content) and publish (content, note).
type ObjectInput struct {
	Name        string            `json:"name"`
	TeamID      uuid.UUID         `json:"teamId"`
	Description string            `json:"description"`
	KeyType     library.ValueType `json:"keyType"`
	Steps       json.RawMessage   `json:"steps"`
	Rows        json.RawMessage   `json:"rows"`
	ChangeNote  string            `json:"changeNote"`
}

// Content returns the member the kind stores.
func (in ObjectInput) Content(m ContentMember) json.RawMessage {
	if m == MemberRows {
		return in.Rows
	}
	return in.Steps
}

// PatchInput is the body of PATCH /{id} (If-Match).
type PatchInput struct {
	Description *string `json:"description"`
}

// ItemDTO is one library object in listings and write responses.
type ItemDTO struct {
	ID            uuid.UUID          `json:"id"`
	Kind          library.ObjectKind `json:"kind"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	KeyType       library.ValueType  `json:"keyType,omitempty"`
	TeamID        uuid.UUID          `json:"teamId"`
	TeamName      string             `json:"teamName"`
	LatestVersion int                `json:"latestVersion"`
	Size          int                `json:"size"`
	UsageCount    int                `json:"usageCount"`
	ArchivedAt    *time.Time         `json:"archivedAt"`
	RowVersion    store.RowVersion   `json:"rowVersion"`
	UpdatedAt     time.Time          `json:"updatedAt"`
	UpdatedBy     string             `json:"updatedBy"`
}

// VersionDTO is one immutable version; Content is keyed by the kind's member (steps / rows).
type VersionDTO map[string]any

// UsageDTO is one route referencing an object.
type UsageDTO struct {
	RouteID   uuid.UUID `json:"routeId"`
	RouteName string    `json:"routeName"`
	Versions  []int     `json:"versions"`
	InDraft   bool      `json:"inDraft"`
	InActive  bool      `json:"inActive"`
}

// RowsDTO is one page of lookup rows.
type RowsDTO struct {
	Items      []library.LookupRow `json:"items"`
	Total      int                 `json:"total"`
	NextCursor *string             `json:"next_cursor"`
}

// ListDTO wraps non-paged listings.
type ListDTO[T any] struct {
	Items []T `json:"items"`
}

// Presenter renders library objects.
type Presenter struct{}

// Item renders one object.
func (Presenter) Item(it library.Item) ItemDTO {
	return ItemDTO{ID: it.ID, Kind: it.Kind, Name: it.Name, Description: it.Description, KeyType: it.KeyType, TeamID: it.TeamID,
		TeamName: it.TeamName, LatestVersion: it.LatestVersion, Size: it.Size, UsageCount: it.UsageCount, ArchivedAt: it.ArchivedAt,
		RowVersion: it.RowVersion, UpdatedAt: it.UpdatedAt, UpdatedBy: it.UpdatedBy}
}

// Items renders a list.
func (p Presenter) Items(items []library.Item) []ItemDTO {
	out := make([]ItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, p.Item(it))
	}
	return out
}

// Version renders one version with its content under the kind's member.
func (Presenter) Version(v library.Version, m ContentMember) VersionDTO {
	return VersionDTO{string(VersionMemberNo): v.No, string(m): v.Content, string(VersionMemberSize): v.Size,
		string(VersionMemberHash): v.ConfigHash, string(VersionMemberNote): v.ChangeNote,
		string(VersionMemberCreatedAt): v.CreatedAt, string(VersionMemberCreatedBy): v.CreatedBy}
}

// Usages renders usages.
func (Presenter) Usages(usages []library.Usage) []UsageDTO {
	out := make([]UsageDTO, 0, len(usages))
	for _, u := range usages {
		versions := u.Versions
		if versions == nil {
			versions = []int{}
		}
		out = append(out, UsageDTO{RouteID: u.RouteID, RouteName: u.RouteName, Versions: versions, InDraft: u.InDraft, InActive: u.InActive})
	}
	return out
}

// VersionMember names the fixed members of VersionDTO.
type VersionMember string

const (
	VersionMemberNo        VersionMember = "version"
	VersionMemberSize      VersionMember = "size"
	VersionMemberHash      VersionMember = "configHash"
	VersionMemberNote      VersionMember = "changeNote"
	VersionMemberCreatedAt VersionMember = "createdAt"
	VersionMemberCreatedBy VersionMember = "createdBy"
)

// ItemAudit is the audit "after" of a write: name, version and metadata, never the content.
type ItemAudit struct {
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Size        int    `json:"size"`
	Description string `json:"description,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
}
