package libraryhttp

import (
	"mime"
	"net/http"
	"strconv"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/platform/api"
)

// VersionsHandler serves GET /api/v1/{kind}/{id}/versions (newest first).
type VersionsHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *VersionsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.k.Versions), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *VersionsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	versions, err := h.d.Service.Versions(r.Context(), h.k.Kind, id)
	if err != nil {
		return err
	}
	out := make([]VersionDTO, 0, len(versions))
	for _, v := range versions {
		out = append(out, Presenter{}.Version(v, h.k.Content))
	}
	return h.d.Responder.JSON(w, http.StatusOK, ListDTO[VersionDTO]{Items: out})
}

// VersionHandler serves GET /api/v1/{kind}/{id}/versions/{version}.
type VersionHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *VersionHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.k.Version), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *VersionHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	no, err := VersionNo{}.Of(r)
	if err != nil {
		return err
	}
	v, err := h.d.Service.Version(r.Context(), h.k.Kind, id, no)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, Presenter{}.Version(v, h.k.Content))
}

// PublishHandler serves POST /api/v1/{kind}/{id}/versions: JSON {steps|rows, changeNote}, or for lookup
// tables a text/csv body (?changeNote=) imported line by line.
type PublishHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler (library.write on the object's team).
func (h *PublishHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(h.k.Versions), Permission: PermWrite, Object: h.d.OwnedObject(h.k),
		Mutating: true, AuditAction: h.k.Published, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *PublishHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	author := auth.PrincipalFrom(r.Context()).UserID
	var it library.Item
	if h.imports(r) {
		body := http.MaxBytesReader(w, r.Body, int64(api.BodyLimitLarge))
		it, err = h.d.Service.Import(r.Context(), id, r.Header.Get(string(HeaderContentType)), body,
			library.VersionDraft{ChangeNote: r.URL.Query().Get(string(QueryChangeNote)), Author: author})
	} else {
		var in ObjectInput
		if in, err = (api.Decoder[ObjectInput]{Limit: api.BodyLimitLarge}).Decode(w, r); err == nil {
			it, err = h.d.Service.Publish(r.Context(), h.k.Kind, id, library.VersionDraft{Content: in.Content(h.k.Content), ChangeNote: in.ChangeNote, Author: author})
		}
	}
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, h.k.Published, it); err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, it.RowVersion, Presenter{}.Item(it))
}

// imports reports whether the body is a file import (kinds with imports, any non-JSON media type).
func (h *PublishHandler) imports(r *http.Request) bool {
	media, _, _ := mime.ParseMediaType(r.Header.Get(string(HeaderContentType)))
	return h.k.Imports && media != string(library.ImportJSON)
}

// Header names request headers read by the module.
type Header string

const HeaderContentType Header = "Content-Type"

// UsagesHandler serves GET /api/v1/{kind}/{id}/usages.
type UsagesHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *UsagesHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.k.Usages), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *UsagesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	usages, err := h.d.Service.Usages(r.Context(), h.k.Kind, id)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ListDTO[UsageDTO]{Items: Presenter{}.Usages(usages)})
}

// RowsHandler serves GET /api/v1/lookup-tables/{id}/versions/{version}/rows?cursor&limit.
type RowsHandler struct {
	d Deps
}

// Route implements api.Handler.
func (h *RowsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathLookupTableRows), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *RowsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	no, err := VersionNo{}.Of(r)
	if err != nil {
		return err
	}
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	q := library.RowsQuery{Version: no, Limit: page.Limit}
	if page.After != nil {
		q.Offset, _ = strconv.Atoi(page.After.ID)
	}
	res, err := h.d.Service.Rows(r.Context(), id, q)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, RowsDTO{Items: res.Rows, Total: res.Total, NextCursor: h.d.Pages.Next(res.Next)})
}
