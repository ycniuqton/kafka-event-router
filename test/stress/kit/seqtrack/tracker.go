// Package seqtrack checks delivery completeness: per stream key, every sequence number 0..n-1 the producer sent must
// arrive at least once; repeats are duplicates, holes below the highest seen number are gaps.
package seqtrack

// stream is the arrival state of one key: the contiguous watermark plus numbers seen above it.
type stream struct {
	next    int64
	maxSeen int64
	ahead   map[int64]struct{}
}

// Tracker accumulates arrivals of many streams; not safe for concurrent use.
type Tracker struct {
	streams    map[string]*stream
	received   uint64
	unique     uint64
	duplicates uint64
	outOfOrder uint64
}

// Summary is the completeness verdict of everything observed.
type Summary struct {
	Streams    int    `json:"streams"`
	Received   uint64 `json:"received"`
	Unique     uint64 `json:"unique"`
	Duplicates uint64 `json:"duplicates"`
	Gaps       uint64 `json:"gaps"`
	OutOfOrder uint64 `json:"outOfOrder"`
	Expected   uint64 `json:"expected,omitempty"`
	Lost       int64  `json:"lost,omitempty"`
}

// New returns an empty tracker.
func New() *Tracker {
	return &Tracker{streams: map[string]*stream{}}
}

// Observe records the arrival of sequence number seq on stream key.
func (t *Tracker) Observe(key string, seq int64) {
	t.received++
	s := t.streams[key]
	if s == nil {
		s = &stream{maxSeen: -1}
		t.streams[key] = s
	}
	if s.seen(seq) {
		t.duplicates++
		return
	}
	if seq < s.maxSeen {
		t.outOfOrder++
	}
	t.unique++
	s.accept(seq)
}

// Unique is the number of distinct (stream, seq) pairs seen so far.
func (t *Tracker) Unique() uint64 {
	return t.unique
}

// Summary returns totals; expected (0 = unknown) is the number of distinct events the producer acknowledged.
func (t *Tracker) Summary(expected uint64) Summary {
	out := Summary{Streams: len(t.streams), Received: t.received, Duplicates: t.duplicates, OutOfOrder: t.outOfOrder}
	for _, s := range t.streams {
		unique := uint64(s.next) + uint64(len(s.ahead))
		out.Unique += unique
		out.Gaps += uint64(s.maxSeen+1) - unique
	}
	if expected > 0 {
		out.Expected = expected
		out.Lost = int64(expected) - int64(out.Unique)
	}
	return out
}

func (s *stream) seen(seq int64) bool {
	if seq < s.next {
		return true
	}
	_, ok := s.ahead[seq]
	return ok
}

func (s *stream) accept(seq int64) {
	s.maxSeen = max(s.maxSeen, seq)
	if seq != s.next {
		if s.ahead == nil {
			s.ahead = map[int64]struct{}{}
		}
		s.ahead[seq] = struct{}{}
		return
	}
	s.next++
	for len(s.ahead) > 0 {
		if _, ok := s.ahead[s.next]; !ok {
			return
		}
		delete(s.ahead, s.next)
		s.next++
	}
}
