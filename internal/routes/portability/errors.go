package portability

import (
	"fmt"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ErrorPath names request members portability errors point at.
type ErrorPath string

const (
	PathFormat  ErrorPath = "/format"
	PathVersion ErrorPath = "/version"
	PathMode    ErrorPath = "/mode"
	PathIDs     ErrorPath = "/ids"
	PathBody    ErrorPath = ""
)

// Limit bounds import and export requests.
type Limit int

const (
	MaxDocuments Limit = 200
	MaxIDs       Limit = 200
)

var (
	// ErrUnknownFormat is returned for formats other than yaml and json (400).
	ErrUnknownFormat = problem.NewError(problem.KindMalformedBody, "Unknown format. Use yaml or json.").At(string(PathFormat))
	// ErrBadImportMode is returned for import modes other than create and upsert (400).
	ErrBadImportMode = problem.NewError(problem.KindMalformedBody, "Unknown import mode. Use create or upsert.").At(string(PathMode))
	// ErrNoDocuments is returned when an import payload holds no document (422).
	ErrNoDocuments = problem.NewError(problem.KindValidation, "The file holds no route document. Paste or upload an exported route.").At(string(PathBody))
	// ErrTooManyDocuments is returned above MaxDocuments (422).
	ErrTooManyDocuments = problem.NewError(problem.KindValidation, fmt.Sprintf("Import at most %d routes at once.", MaxDocuments)).At(string(PathBody))
	// ErrNoIDs is returned for an export without ids (422).
	ErrNoIDs = problem.NewError(problem.KindValidation, "Select at least one route to export.").At(string(PathIDs))
	// ErrTooManyIDs is returned above MaxIDs (422).
	ErrTooManyIDs = problem.NewError(problem.KindValidation, fmt.Sprintf("Export at most %d routes at once.", MaxIDs)).At(string(PathIDs))
	// ErrNothingToExport is returned when the requested version side does not exist (404).
	ErrNothingToExport = problem.NewError(problem.KindNotFound, "This route has no configuration to export for that version.").At(string(PathVersion))
)
