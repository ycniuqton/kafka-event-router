package membudget

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/twmb/franz-go/pkg/kgo"
)

// aliases reports whether b points into the response buffer resp.
func aliases(b, resp []byte) bool {
	if len(b) == 0 {
		return false
	}
	p, lo := uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&resp[0]))
	return p >= lo && p < lo+uintptr(len(resp))
}

func TestRecordDetachCopiesOutOfTheFetchResponse(t *testing.T) {
	resp := bytes.Repeat([]byte("x"), 1<<20)
	copy(resp, "k1v1h1")
	recs := []*kgo.Record{
		{Key: resp[0:2], Value: resp[2:4], Headers: []kgo.RecordHeader{{Key: "h", Value: resp[4:6]}}},
		{Key: nil, Value: resp[100:110], Headers: []kgo.RecordHeader{{Key: "empty", Value: nil}}},
	}
	before := RecordsCost(recs)
	RecordDetach{}.Detach(recs)
	r := recs[0]
	if string(r.Key) != "k1" || string(r.Value) != "v1" || string(r.Headers[0].Value) != "h1" || r.Headers[0].Key != "h" {
		t.Fatalf("content changed: %q %q %+v", r.Key, r.Value, r.Headers)
	}
	for _, b := range [][]byte{r.Key, r.Value, r.Headers[0].Value, recs[1].Value} {
		if aliases(b, resp) {
			t.Fatal("a detached field still points into the fetch response")
		}
	}
	if recs[1].Key != nil || recs[1].Headers[0].Value != nil {
		t.Fatal("nil key / header value must stay nil")
	}
	if RecordsCost(recs) != before {
		t.Fatalf("detaching must not change the charged cost: %d → %d", before, RecordsCost(recs))
	}
	r.Key = append(r.Key, 'z')
	if string(r.Value) != "v1" {
		t.Fatal("fields of one record must not overlap (append to the key overwrote the value)")
	}
}

func BenchmarkRecordDetach1KB(b *testing.B) {
	resp := make([]byte, 1<<20)
	recs := make([]*kgo.Record, 500)
	for i := range recs {
		recs[i] = &kgo.Record{Key: resp[i*1100 : i*1100+16], Value: resp[i*1100+16 : i*1100+1040],
			Headers: []kgo.RecordHeader{{Key: "stress-seq", Value: resp[i*1100+1040 : i*1100+1048]}}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordDetach{}.one(recs[i%len(recs)])
		recs[i%len(recs)].Value = resp[(i%len(recs))*1100+16 : (i%len(recs))*1100+1040]
	}
}
