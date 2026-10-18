// Package handlers exposes secrets to workloads on the internal listener only; the public API never returns them.
package handlers

import (
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/secrets"
)

// InternalPath names the internal secrets routes and params.
type InternalPath string

const (
	PathSecretByRef InternalPath = "/internal/v1/secrets/{ref}"
	ParamRef        InternalPath = "ref"
)

// SecretResponse is the internal read body; value is base64 of the plaintext.
type SecretResponse struct {
	Ref      string `json:"ref"`
	ValueB64 string `json:"value_b64"`
}

// SecretReadHandler serves GET /internal/v1/secrets/{ref} to authenticated workloads, audited per read.
type SecretReadHandler struct {
	reader    *secrets.AuditedReader
	responder *api.Responder
}

// NewModule returns the internal secrets module.
func NewModule(reader *secrets.AuditedReader, responder *api.Responder) api.Module {
	return api.Modules{&SecretReadHandler{reader: reader, responder: responder}}
}

// Route implements api.Handler.
func (h *SecretReadHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathSecretByRef), Auth: api.AuthRequired}
}

// Serve implements api.Handler.
func (h *SecretReadHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	ref := secrets.Ref(chi.URLParam(r, string(ParamRef)))
	if !ref.Valid() {
		return secrets.ErrNotFound
	}
	plain, err := h.reader.Read(r.Context(), ref, api.AuditContext(r))
	if err != nil {
		return err
	}
	defer clear(plain)
	w.Header().Set(string(api.HeaderCacheControl), string(api.CacheNoStore))
	return h.responder.JSON(w, http.StatusOK, SecretResponse{Ref: string(ref), ValueB64: base64.StdEncoding.EncodeToString(plain)})
}
