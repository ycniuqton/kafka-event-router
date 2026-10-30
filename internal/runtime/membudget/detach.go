package membudget

import "github.com/twmb/franz-go/pkg/kgo"

// RecordDetach copies polled records out of their fetch response: franz-go records alias the whole response
// buffer (every partition of one fetch), so a single record queued in a slow lane would pin megabytes the
// budget never charged (wave-2 overload: 92 % of the heap was fetch buffers at 25 fan-out routes).
type RecordDetach struct{}

// Detach gives every record its own copy of key, value and header values (one allocation per record).
func (RecordDetach) Detach(recs []*kgo.Record) {
	for _, r := range recs {
		RecordDetach{}.one(r)
	}
}

// one copies r's byte fields into one fresh buffer; nil fields stay nil (a nil key is not an empty key).
func (RecordDetach) one(r *kgo.Record) {
	size := len(r.Key) + len(r.Value)
	for _, h := range r.Headers {
		size += len(h.Value)
	}
	buf := make([]byte, 0, size)
	r.Key, buf = detachedField{}.take(buf, r.Key)
	r.Value, buf = detachedField{}.take(buf, r.Value)
	headers := make([]kgo.RecordHeader, len(r.Headers))
	for i, h := range r.Headers {
		headers[i].Key = h.Key
		headers[i].Value, buf = detachedField{}.take(buf, h.Value)
	}
	r.Headers = headers
}

// detachedField appends one field to the record's private buffer.
type detachedField struct{}

// take copies src to the end of buf and returns the copy (nil for nil) and the grown buffer.
func (detachedField) take(buf, src []byte) ([]byte, []byte) {
	if src == nil {
		return nil, buf
	}
	start := len(buf)
	buf = append(buf, src...)
	return buf[start:len(buf):len(buf)], buf
}
