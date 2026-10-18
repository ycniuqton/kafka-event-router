package step

import (
	"slices"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryDepth bounds how deep library transforms may include other library transforms.
type LibraryDepth int

const LibraryMaxDepth LibraryDepth = 4

// LibraryStack is the chain of library refs being expanded, outermost first.
type LibraryStack []string

// Contains reports whether ref is already being expanded (a cycle).
func (s LibraryStack) Contains(ref string) bool { return slices.Contains(s, ref) }

// Full reports whether one more expansion would exceed LibraryMaxDepth.
func (s LibraryStack) Full() bool { return LibraryDepth(len(s)) >= LibraryMaxDepth }

// Push returns a new stack with ref on top; the receiver is not modified.
func (s LibraryStack) Push(ref string) LibraryStack { return append(slices.Clone(s), ref) }

// Splicer is a compiled step that expands into several chain entries at compile time
// (library), so each expanded step is traced and reported on its own.
type Splicer interface {
	Step
	Entries() []ChainEntry
}

// ChainEntries turns one compiled step into the entries it contributes to a chain.
type ChainEntries struct{}

// Of returns the spliced entries of a Splicer, else the step itself at ptr.
func (ChainEntries) Of(s Step, ptr config.Pointer) []ChainEntry {
	if sp, ok := s.(Splicer); ok {
		return sp.Entries()
	}
	return []ChainEntry{{Step: s, Pointer: ptr}}
}
