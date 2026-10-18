//go:build linux

package quickjs

import (
	"syscall"

	"github.com/tetratelabs/wazero/experimental"
)

// mmapAllocator backs each guest memory with an anonymous mapping of its maximum size:
// growing is free, untouched pages cost no RSS and Free returns memory to the OS at once.
type mmapAllocator struct{}

func (mmapAllocator) Allocate(_, max uint64) experimental.LinearMemory {
	buf, err := syscall.Mmap(-1, 0, int(max), syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_NORESERVE)
	if err != nil {
		return nil
	}
	return &mmapMemory{buf: buf}
}

// mmapMemory is one mapping handed to wazero.
type mmapMemory struct {
	buf []byte
}

func (m *mmapMemory) Reallocate(size uint64) []byte {
	if size > uint64(len(m.buf)) {
		return nil
	}
	return m.buf[:size]
}

func (m *mmapMemory) Free() {
	if m.buf != nil {
		_ = syscall.Munmap(m.buf)
		m.buf = nil
	}
}

// guestMemoryAllocator returns the allocator used for guest linear memories.
func guestMemoryAllocator() experimental.MemoryAllocator { return mmapAllocator{} }
