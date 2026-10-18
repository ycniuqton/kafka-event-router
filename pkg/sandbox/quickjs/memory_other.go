//go:build !linux

package quickjs

import "github.com/tetratelabs/wazero/experimental"

// guestMemoryAllocator returns nil so wazero uses its default Go-heap memory.
func guestMemoryAllocator() experimental.MemoryAllocator { return nil }
