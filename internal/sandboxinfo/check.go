package sandboxinfo

import (
	"context"
	"net/http"
	"sync"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
)

// PathCheck is the editor's compile check (js-sandbox design §5).
const PathCheck Path = "/api/v1/sandbox/check"

// CheckText names user-facing texts and pointers of the check endpoint.
type CheckText string

const (
	PointerCode        CheckText = "/code"
	PointerEntry       CheckText = "/entry"
	CodeEntryUnknown   CheckText = "entry_unknown"
	DetailCodeTooLarge CheckText = "The code is larger than the sandbox source limit. Split it or shorten it."
	DetailEntryUnknown CheckText = "entry must be \"transform\" or \"predicate\"."
	DetailCheckOff     CheckText = "The JS sandbox is not available on this server; code cannot be checked."
)

// CheckBodyFactor sizes the body limit: JSON escaping can grow the code up to 6 bytes per byte.
type CheckBodyFactor int64

const checkBodyFactor CheckBodyFactor = 6

// SourceChecker compiles user code without running events; an error means no engine is available.
type SourceChecker interface {
	Check(ctx context.Context, src sandbox.Source) ([]sandbox.Diagnostic, error)
}

// ManagerChecker checks code with an already running sandbox Manager.
type ManagerChecker struct {
	Manager sandbox.Manager
}

func (c ManagerChecker) Check(ctx context.Context, src sandbox.Source) ([]sandbox.Diagnostic, error) {
	return c.Manager.Check(ctx, src), nil
}

// CheckRequest is the body of POST /api/v1/sandbox/check; entry defaults to transform.
type CheckRequest struct {
	Code  string            `json:"code"`
	Entry sandbox.EntryKind `json:"entry,omitempty"`
}

// CheckResponse lists the diagnostics of the code (empty when it compiles).
type CheckResponse struct {
	Diagnostics []sandbox.Diagnostic `json:"diagnostics"`
}

// knownEntries are the entry kinds the check accepts.
var knownEntries = map[sandbox.EntryKind]bool{sandbox.EntryTransform: true, sandbox.EntryPredicate: true}

// CheckHandler serves POST /api/v1/sandbox/check: 413 above the source limit, 422 for an unknown entry.
type CheckHandler struct {
	checker   SourceChecker
	maxSource int64
	responder *api.Responder
}

// NewCheckHandler builds the handler; maxSource is the sandbox MaxSourceBytes.
func NewCheckHandler(checker SourceChecker, maxSource int64, responder *api.Responder) *CheckHandler {
	return &CheckHandler{checker: checker, maxSource: maxSource, responder: responder}
}

// Route implements api.Handler.
func (h *CheckHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathCheck), Permission: PermRuntimeInfo, BodyLimit: h.bodyLimit()}
}

func (h *CheckHandler) bodyLimit() api.BodyLimit {
	return api.BodyLimit(h.maxSource * int64(checkBodyFactor))
}

// Serve implements api.Handler.
func (h *CheckHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[CheckRequest]{Limit: h.bodyLimit()}.Decode(w, r)
	if err != nil {
		return err
	}
	if req.Entry == "" {
		req.Entry = sandbox.EntryTransform
	}
	if !knownEntries[req.Entry] {
		return api.NewValidationError().Add(string(PointerEntry), string(CodeEntryUnknown), string(DetailEntryUnknown))
	}
	if int64(len(req.Code)) > h.maxSource {
		return problem.NewError(problem.KindPayloadTooLarge, string(DetailCodeTooLarge)).At(string(PointerCode))
	}
	diags, err := h.checker.Check(r.Context(), sandbox.Source{Entry: req.Entry, Code: req.Code})
	if err != nil {
		return problem.NewError(problem.KindUnavailable, string(DetailCheckOff)).Wrap(err)
	}
	if diags == nil {
		diags = []sandbox.Diagnostic{}
	}
	return h.responder.JSON(w, http.StatusOK, CheckResponse{Diagnostics: diags})
}

// CheckSaltText is the fixed salt of editor checks: they compile code but never hash event data.
type CheckSaltText string

const checkSalt CheckSaltText = "sandbox-editor-check"

// CheckSalts returns the fixed check salt for every team.
type CheckSalts struct{}

func (CheckSalts) Salt(context.Context, string) ([]byte, error) { return []byte(checkSalt), nil }

// LazyChecker boots a sandbox Manager on the first check, so API start-up never pays for the engine.
type LazyChecker struct {
	Config  sandbox.Config
	once    sync.Once
	manager sandbox.Manager
	err     error
}

// Check implements SourceChecker; an engine that failed to boot keeps failing (503).
func (l *LazyChecker) Check(ctx context.Context, src sandbox.Source) ([]sandbox.Diagnostic, error) {
	l.once.Do(func() { l.manager, l.err = l.boot(ctx) })
	if l.err != nil {
		return nil, l.err
	}
	return l.manager.Check(ctx, src), nil
}

func (l *LazyChecker) boot(ctx context.Context) (sandbox.Manager, error) {
	return sandbox.NewManager(context.WithoutCancel(ctx), l.Config, sandbox.ManagerDeps{
		Engines: engines.Registry(), Salts: CheckSalts{}, Binder: enginebind.Binder{Base: ctxhelpers.Set{}},
	})
}

// Close releases the engine if it was booted.
func (l *LazyChecker) Close(ctx context.Context) error {
	if l.manager == nil {
		return nil
	}
	return l.manager.Close(ctx)
}
