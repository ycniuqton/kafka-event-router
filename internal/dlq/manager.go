package dlq

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq/record"
)

// ManagerDeps are the components the Manager façade composes.
type ManagerDeps struct {
	Routes    RouteSource
	Locator   Locator
	Reader    Reader
	State     StateStore
	Actions   []ActionHandler
	Exports   *ExportManager
	Views     ViewPolicy
	Payloads  PayloadDecoder
	Studio    StudioLinker
	Summaries SummaryCache
	Groups    GroupCache
	Gate      ReplayGate
	Audit     AuditTrail
	Metrics   *Metrics
	Log       *slog.Logger
	Now       func() time.Time
}

// Manager is the façade the HTTP handlers use (design §2.2).
type Manager struct {
	d          ManagerDeps
	gate       StateGate
	selector   Selector
	actions    map[ActionKind]ActionHandler
	groupsOnce sync.Once
}

// NewManager builds the façade.
func NewManager(d ManagerDeps) *Manager {
	if d.Payloads == nil {
		d.Payloads = PlainPayloads{}
	}
	gate := StateGate{Store: d.State}
	m := &Manager{d: d, gate: gate, selector: Selector{Reader: d.Reader, Gate: gate, Now: d.Now}, actions: map[ActionKind]ActionHandler{}}
	for _, a := range d.Actions {
		m.actions[a.Kind()] = a
	}
	return m
}

// ListPage is one page of list rows.
type ListPage struct {
	Items      []ItemView     `json:"items"`
	NextCursor *string        `json:"next_cursor"`
	Scanned    int            `json:"scanned"`
	Truncated  bool           `json:"truncated"`
	Warnings   []RouteWarning `json:"warnings,omitempty"`
}

// RouteWarning is a warning about one route of a list.
type RouteWarning struct {
	RouteID string  `json:"routeId,omitempty"`
	Reason  Warning `json:"reason"`
}

// Routes returns every route (the handler filters by permission).
func (m *Manager) Routes(ctx context.Context) ([]RouteInfo, error) {
	return m.d.Routes.Routes(ctx)
}

// Route returns one route (job authorization, job notifications).
func (m *Manager) Route(ctx context.Context, id string) (RouteInfo, error) {
	return m.d.Routes.Route(ctx, id)
}

// List returns a page of a route's unresolved dead letters, newest first (design §5.1).
func (m *Manager) List(ctx context.Context, routeID string, q Query) (ListPage, error) {
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return ListPage{}, err
	}
	cur, err := CursorCodec{}.Decode(q.Cursor)
	if err != nil {
		return ListPage{}, err
	}
	page, err := m.scan(ctx, loc, q, cur, q.PageSize())
	if err != nil {
		return ListPage{}, err
	}
	items, err := m.items(ctx, loc, page.Records, q.IncludeResolved)
	if err != nil {
		return ListPage{}, err
	}
	out := ListPage{Items: items, Scanned: page.Scanned, Truncated: page.Truncated, Warnings: (WarningsOf{}).Missing("", page.Missing)}
	if !page.Next.Done {
		next := CursorCodec{}.Encode(page.Next)
		out.NextCursor = &next
	}
	return out, nil
}

// scan runs one budgeted scan of a location, filtering resolved records.
func (m *Manager) scan(ctx context.Context, loc Location, q Query, cur Cursor, want int) (ScanPage, error) {
	if len(loc.Targets) == 0 {
		return ScanPage{Next: Cursor{Done: true}}, nil
	}
	start := m.d.Now()
	page, err := m.d.Reader.Scan(ctx, loc.Targets, ScanRequest{
		Query: q, Cursor: cur, Want: want, Budget: NewScanBudget(m.d.Now),
		Filter: StateFilter{Gate: m.gate, Location: loc, Include: q.IncludeResolved},
	})
	m.d.Metrics.Scan(m.d.Now().Sub(start), page.Scanned)
	return page, err
}

// items renders rows, attaching resolutions when resolved rows are included.
func (m *Manager) items(ctx context.Context, loc Location, recs []record.Record, withResolution bool) ([]ItemView, error) {
	resolutions := Resolutions{}
	if withResolution {
		var err error
		if resolutions, err = m.resolutions(ctx, loc, recs); err != nil {
			return nil, err
		}
	}
	out := make([]ItemView, 0, len(recs))
	for _, r := range recs {
		out = append(out, loc.Presenter().Item(loc.Route, r, resolutions.Of(r.Ref)))
	}
	return out, nil
}

// resolutions looks up the state of recs on their connections.
func (m *Manager) resolutions(ctx context.Context, loc Location, recs []record.Record) (Resolutions, error) {
	out := Resolutions{}
	for conn, group := range (ByConnection{}).Records(loc, recs) {
		found, err := m.d.State.Lookup(ctx, conn, loc.Route.ID, (ByConnection{}).Refs(group))
		if err != nil {
			return nil, err
		}
		for k, v := range found {
			out[k] = v
		}
	}
	return out, nil
}

// Get returns the detail of one dead letter and audits the view with its masked flag (design §5.4).
func (m *Manager) Get(ctx context.Context, viewer Viewer, routeID, id string) (DetailView, error) {
	ref, err := RefCodec{}.Decode(id)
	if err != nil {
		return DetailView{}, ErrInvalidID.At(string(PathDLQID))
	}
	loc, rec, err := m.fetchOne(ctx, routeID, ref)
	if err != nil {
		return DetailView{}, err
	}
	resolutions, err := m.resolutions(ctx, loc, []record.Record{rec})
	if err != nil {
		return DetailView{}, err
	}
	d, err := m.detail(ctx, viewer, loc, rec, resolutions.Of(ref))
	if err != nil {
		return DetailView{}, err
	}
	detail := ViewAuditDetail{ID: id, Masked: d.Masked}
	return d, m.d.Audit.Record(ctx, AuditEvent{Action: audit.ActionDLQViewed, Route: loc.Route, Detail: detail, Masked: d.Masked})
}

// detail renders one record: plain view, schema decoding, then the viewer's masking policy.
func (m *Manager) detail(ctx context.Context, viewer Viewer, loc Location, rec record.Record, res *Resolution) (DetailView, error) {
	target, _ := loc.TargetOf(rec.Ref.Topic)
	d := m.d.Payloads.Decode(ctx, loc.Formats, rec, loc.Presenter().Detail(loc.Route, rec, res))
	return m.d.Views.Present(ctx, viewer, target.ConnectionID, d)
}

// fetchOne reads one record of a route; foreign topics are not found, old offsets expired.
func (m *Manager) fetchOne(ctx context.Context, routeID string, ref Ref) (Location, record.Record, error) {
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return Location{}, record.Record{}, err
	}
	if _, ok := loc.TargetOf(ref.Topic); !ok {
		return Location{}, record.Record{}, ErrRecordNotFound
	}
	res, err := m.d.Reader.Fetch(ctx, loc.Targets, []Ref{ref})
	if err != nil {
		return Location{}, record.Record{}, err
	}
	if len(res.Expired) > 0 {
		return Location{}, record.Record{}, ErrRecordExpired
	}
	rec, ok := res.Records[ref]
	if !ok {
		return Location{}, record.Record{}, ErrRecordNotFound
	}
	return loc, rec, nil
}

// OpenInStudio creates a sample session with the failing event (design §5.9).
func (m *Manager) OpenInStudio(ctx context.Context, viewer Viewer, routeID, id string) (StudioLink, error) {
	ref, err := RefCodec{}.Decode(id)
	if err != nil {
		return StudioLink{}, ErrInvalidID.At(string(PathDLQID))
	}
	loc, rec, err := m.fetchOne(ctx, routeID, ref)
	if err != nil {
		return StudioLink{}, err
	}
	target, _ := loc.TargetOf(ref.Topic)
	shown, masked, err := m.d.Views.PresentRecord(ctx, viewer, target.ConnectionID, rec)
	if err != nil {
		return StudioLink{}, err
	}
	return m.d.Studio.Link(ctx, loc.Route, viewer, shown, masked)
}

// WarningsOf builds warning lists.
type WarningsOf struct{}

// Missing returns one dlq_topic_missing warning when topics are missing.
func (WarningsOf) Missing(routeID string, missing []string) []RouteWarning {
	if len(missing) == 0 {
		return nil
	}
	return []RouteWarning{{RouteID: routeID, Reason: WarnTopicMissing}}
}
