package routeshttp

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/portability"
)

// Portability paths and query keys.
const (
	PathExport      Path        = "/api/v1/routes/{id}/export"
	PathBulkExport  Path        = "/api/v1/routes/export"
	PathImport      Path        = "/api/v1/routes/import"
	QueryFormat     QueryKey    = "format"
	QueryVersion    QueryKey    = "version"
	QueryMode       QueryKey    = "mode"
	QueryDryRun     QueryKey    = "dryRun"
	HeaderTypeKey   HeaderName  = "Content-Type"
	HeaderAttach    HeaderName  = "Content-Disposition"
	attachmentStart Disposition = "attachment; filename="
)

// HeaderName names HTTP headers set by the portability handlers.
type HeaderName string

// Disposition is the Content-Disposition prefix of downloads.
type Disposition string

// ExportBody is the body of POST /routes/export.
type ExportBody struct {
	IDs     []string `json:"ids"`
	Format  string   `json:"format"`
	Version string   `json:"version"`
}

// ImportEnvelope is the JSON form of an import body (browser clients): the file text plus its format.
type ImportEnvelope struct {
	Content string `json:"content"`
	Format  string `json:"format"`
}

// ArtifactWriter sends an export as a file download.
type ArtifactWriter struct{}

// Write sets the media type and attachment name and writes the content.
func (ArtifactWriter) Write(w http.ResponseWriter, a portability.Artifact) error {
	w.Header().Set(string(HeaderTypeKey), a.MediaType)
	w.Header().Set(string(HeaderAttach), string(attachmentStart)+strconv.Quote(a.FileName))
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, a.Content)
	return err
}

// ExportHandler serves GET /routes/{id}/export?format=yaml|json&version=active|draft|N.
type ExportHandler struct{ d Deps }

func (h *ExportHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathExport), Permission: PermRead, Object: RouteObject}
}

func (h *ExportHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	a, err := h.d.Portability.Export(r.Context(), portability.ExportRequest{IDs: []routes.RouteID{id},
		Format: routes.DocumentFormat(q.Get(string(QueryFormat))), Version: q.Get(string(QueryVersion))})
	if err != nil {
		return err
	}
	return ArtifactWriter{}.Write(w, a)
}

// BulkExportHandler serves POST /routes/export {ids, format, version?} (multi-document download, audited).
type BulkExportHandler struct{ d Deps }

func (h *BulkExportHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathBulkExport), Permission: PermRead,
		Mutating: true, AuditAction: routes.ActionRouteExported}
}

func (h *BulkExportHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[ExportBody]{}.Decode(w, r)
	if err != nil {
		return err
	}
	ids := make([]routes.RouteID, len(body.IDs))
	for i, id := range body.IDs {
		ids[i] = routes.RouteID(id)
	}
	a, err := h.d.Portability.Export(r.Context(), portability.ExportRequest{IDs: ids, Format: routes.DocumentFormat(body.Format), Version: body.Version})
	if err != nil {
		return err
	}
	return ArtifactWriter{}.Write(w, a)
}

// ImportHandler serves POST /routes/import?mode=create|upsert&dryRun=bool&teamId= (YAML, JSON or envelope).
type ImportHandler struct{ d Deps }

func (h *ImportHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathImport), Permission: PermWrite,
		Mutating: true, AuditAction: routes.ActionRouteImported, BodyLimit: api.BodyLimitLarge}
}

func (h *ImportHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := ImportRequestReader{}.Read(w, r)
	if err != nil {
		return err
	}
	res, err := h.d.Portability.Import(r.Context(), req)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// ImportRequestReader turns the request into an ImportRequest: the body is the payload unless it is a JSON
// envelope; the format comes from ?format=, the envelope or the Content-Type.
type ImportRequestReader struct{}

// ErrBadDryRun is returned for a dryRun value that is not a boolean (400).
var ErrBadDryRun = problem.NewError(problem.KindMalformedBody, "dryRun must be true or false.").At("/" + string(QueryDryRun))

// Read reads the body within the large body limit.
func (rr ImportRequestReader) Read(w http.ResponseWriter, r *http.Request) (portability.ImportRequest, error) {
	q := r.URL.Query()
	dry, err := rr.dryRun(q.Get(string(QueryDryRun)))
	if err != nil {
		return portability.ImportRequest{}, err
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(api.BodyLimitLarge)))
	if err != nil {
		return portability.ImportRequest{}, problem.NewError(problem.KindPayloadTooLarge, string(api.DetailTooLarge))
	}
	format := routes.DocumentFormat(q.Get(string(QueryFormat)))
	if rr.isJSON(r) {
		payload, format = rr.unwrap(payload, format)
	}
	return portability.ImportRequest{Payload: payload, Format: format, Mode: portability.ImportMode(q.Get(string(QueryMode))),
		DryRun: dry, TeamID: routes.TeamID(q.Get(string(QueryTeam)))}, nil
}

func (ImportRequestReader) dryRun(raw string) (bool, error) {
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, ErrBadDryRun
	}
	return v, nil
}

// isJSON reports a JSON Content-Type.
func (ImportRequestReader) isJSON(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get(string(HeaderTypeKey)))
	return strings.HasSuffix(mt, string(jsonMediaSuffix))
}

// MediaSuffix matches JSON media types.
type MediaSuffix string

const jsonMediaSuffix MediaSuffix = "json"

// unwrap returns the envelope content, or the JSON documents themselves (format json).
func (ImportRequestReader) unwrap(payload []byte, format routes.DocumentFormat) ([]byte, routes.DocumentFormat) {
	var env ImportEnvelope
	if json.Unmarshal(payload, &env) == nil && env.Content != "" {
		if format == "" {
			format = routes.DocumentFormat(env.Format)
		}
		return []byte(env.Content), format
	}
	if format == "" {
		format = routes.FormatJSON
	}
	return payload, format
}

// UnarchiveHandler serves POST /routes/{id}/unarchive (archived → paused).
type UnarchiveHandler struct{ d Deps }

func (h *UnarchiveHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathUnarchive), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteUnarchived}
}

func (h *UnarchiveHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	d, err := h.d.Routes.Unarchive(r.Context(), id)
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Detail(w, http.StatusOK, d)
}
