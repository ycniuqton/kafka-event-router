package determinism

import (
	_ "embed"
	"time"
)

//go:embed prelude.js
var preludeSource string

// Version is the determinism contract version stored with saved test cases.
type Version int

const VersionCurrent Version = 1

// PreludeExport is the index of a function in the array the prelude evaluates to.
type PreludeExport uint32

const (
	ExportInvoke   PreludeExport = 0
	ExportDescribe PreludeExport = 1
	ExportLockdown PreludeExport = 2
	ExportCount    PreludeExport = 3
)

// Prelude is the embedded JavaScript that locks down a guest before user code runs.
type Prelude struct {
	Source   string
	FileName string
}

// PreludeFileName is the script name the prelude reports in stack traces.
type PreludeFileName string

const PreludeFile PreludeFileName = "sandbox-prelude.js"

// DefaultPrelude returns the embedded prelude.
func DefaultPrelude() Prelude {
	return Prelude{Source: preludeSource, FileName: string(PreludeFile)}
}

// LoadEpoch is the frozen clock visible to top-level user code while a program loads.
var LoadEpoch = time.Unix(0, 0).UTC()
