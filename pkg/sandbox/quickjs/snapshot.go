package quickjs

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// snapshotGuard is how many bytes past the last live allocation are captured.
type snapshotGuard uint32

const snapshotGuardBytes snapshotGuard = 64

// memorySnapshot is the pristine post-load image of a guest's live heap. Restoring it before
// every call gives each event a fresh JS state (ResetPolicy fresh_context).
type memorySnapshot struct {
	data []byte
	size uint32
}

// captureSnapshot pre-grows the heap by headroom (so calls need no memory.grow), then copies
// every byte below the free top chunk.
func captureSnapshot(ctx context.Context, abi *guestABI, headroom int64) (*memorySnapshot, error) {
	ptr, err := abi.call32(ctx, exMalloc, uint64(headroom))
	if err != nil {
		return nil, err
	}
	mem := abi.mod.Memory()
	used := mem.Size()
	if ptr != 0 {
		used = min(ptr+uint32(snapshotGuardBytes), mem.Size())
		if err := abi.free(ctx, ptr); err != nil {
			return nil, err
		}
	}
	data, err := abi.readBytes(0, used)
	if err != nil {
		return nil, err
	}
	return &memorySnapshot{data: data, size: mem.Size()}, nil
}

// restore rewrites the captured image; bytes above it are free heap and never read as live data.
func (s *memorySnapshot) restore(mem api.Memory) bool {
	view, ok := mem.Read(0, uint32(len(s.data)))
	if !ok {
		return false
	}
	copy(view, s.data)
	return true
}

// grew reports whether a call had to grow memory, which makes the image stale.
func (s *memorySnapshot) grew(mem api.Memory) bool {
	return mem.Size() != s.size
}

// bytes is the Go-side cost of keeping the image.
func (s *memorySnapshot) bytes() int64 {
	return int64(len(s.data))
}
