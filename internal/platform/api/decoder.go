package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// DecodeDetail names user-facing messages of decoding failures.
type DecodeDetail string

const (
	DetailMalformedJSON DecodeDetail = "The request body is not valid JSON. Fix the syntax and retry."
	DetailWrongType     DecodeDetail = "This field has the wrong type. Check the expected format and retry."
	DetailUnknownField  DecodeDetail = "This field is not recognised. Remove it and retry."
	DetailTooLarge      DecodeDetail = "The request body is too large. Send a smaller payload."
	DetailTrailingData  DecodeDetail = "The request body must contain a single JSON value."
	DetailEmptyBody     DecodeDetail = "The request body is empty. Send a JSON object."
)

// unknownFieldPrefix is the encoding/json message prefix for DisallowUnknownFields failures.
const unknownFieldPrefix = "json: unknown field "

// Decoder decodes a JSON body into T with a size limit and unknown-field rejection.
type Decoder[T any] struct {
	Limit      BodyLimit
	AllowEmpty bool
}

// Decode reads r's body; failures are classified problems with a JSON Pointer path.
func (d Decoder[T]) Decode(w http.ResponseWriter, r *http.Request) (T, error) {
	var out T
	limit := d.Limit
	if limit <= 0 {
		limit = BodyLimitDefault
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(limit)))
	dec.DisallowUnknownFields()
	err := dec.Decode(&out)
	if errors.Is(err, io.EOF) && d.AllowEmpty {
		return out, nil
	}
	if err != nil {
		return out, JSONErrorMapper{}.Map(err)
	}
	if dec.More() {
		return out, problem.NewError(problem.KindMalformedBody, string(DetailTrailingData))
	}
	return out, nil
}

// JSONErrorMapper converts encoding/json errors into problems pointing at the offending field.
type JSONErrorMapper struct{}

// Map classifies err.
func (JSONErrorMapper) Map(err error) error {
	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return problem.NewError(problem.KindPayloadTooLarge, string(DetailTooLarge)).Wrap(err)
	case errors.As(err, &typeErr):
		return problem.NewError(problem.KindMalformedBody, string(DetailWrongType)).At(JSONPointer{}.FromDotted(typeErr.Field)).Wrap(err)
	case strings.HasPrefix(err.Error(), unknownFieldPrefix):
		field := strings.Trim(strings.TrimPrefix(err.Error(), unknownFieldPrefix), `"`)
		return problem.NewError(problem.KindMalformedBody, string(DetailUnknownField)).At(JSONPointer{}.FromDotted(field)).Wrap(err)
	case errors.Is(err, io.EOF):
		return problem.NewError(problem.KindMalformedBody, string(DetailEmptyBody)).Wrap(err)
	default:
		return problem.NewError(problem.KindMalformedBody, string(DetailMalformedJSON)).At("").Wrap(err)
	}
}

// JSONPointer builds RFC 6901 pointers.
type JSONPointer struct{}

// FromDotted converts encoding/json's "a.b.0" field path into "/a/b/0".
func (JSONPointer) FromDotted(field string) string {
	if field == "" {
		return ""
	}
	parts := strings.Split(field, ".")
	for i, p := range parts {
		parts[i] = JSONPointer{}.Escape(p)
	}
	return "/" + strings.Join(parts, "/")
}

// Escape escapes one reference token (~ → ~0, / → ~1).
func (JSONPointer) Escape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}
