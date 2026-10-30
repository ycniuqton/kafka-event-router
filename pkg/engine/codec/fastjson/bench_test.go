package fastjson_test

import (
	"testing"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/fastjson"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// benchPayload is the S1 order event of the given size.
func benchPayload(size enginetest.PayloadSize) []byte {
	return enginetest.OrderPayload{Size: size}.Bytes()
}

func BenchmarkScan(b *testing.B) {
	raw := benchPayload(enginetest.Payload1KB)
	var t fastjson.Tape
	var s fastjson.Scanner
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !s.Scan(raw, &t) {
			b.Fatal("scan")
		}
	}
}

func BenchmarkCanonicalWrite(b *testing.B) {
	raw := benchPayload(enginetest.Payload1KB)
	var t fastjson.Tape
	var s fastjson.Scanner
	s.Scan(raw, &t)
	w := fastjson.CanonicalWriter{}
	w.Reset(&t)
	buf := make([]byte, 0, 2*len(raw))
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = w.Append(buf[:0], fastjson.Root)
	}
}

func BenchmarkTreeRoundTrip(b *testing.B) {
	raw := benchPayload(enginetest.Payload1KB)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, _ := codec.TreeReader{}.Read(raw)
		_, _ = codec.TreeWriter{}.Write(v)
	}
}
