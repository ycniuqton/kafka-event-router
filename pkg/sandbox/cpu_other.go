//go:build !unix

package sandbox

import "time"

// processCPUTime is unknown without getrusage.
func processCPUTime() (time.Duration, bool) { return 0, false }
