// Package loadgen is the stress load generator: a paced producer stamping send time and sequence headers, and a
// measurer computing end-to-end latency and delivery completeness from what reaches the output topics.
package loadgen

import (
	"fmt"
	"sort"

	"github.com/twmb/franz-go/pkg/kgo"
)

// AcksMode names the producer acknowledgement level (loadgen --acks).
type AcksMode string

const (
	AcksAll    AcksMode = "all"
	AcksLeader AcksMode = "leader"
	AcksNone   AcksMode = "none"
)

// Codec names a producer batch compression (loadgen --compression).
type Codec string

const (
	CodecNone   Codec = "none"
	CodecLZ4    Codec = "lz4"
	CodecZstd   Codec = "zstd"
	CodecSnappy Codec = "snappy"
	CodecGzip   Codec = "gzip"
)

// ClientOptions maps configured names to franz-go options (registries, no switch on modes).
type ClientOptions struct{}

var acksOptions = map[AcksMode][]kgo.Opt{
	AcksAll:    {kgo.RequiredAcks(kgo.AllISRAcks())},
	AcksLeader: {kgo.RequiredAcks(kgo.LeaderAck()), kgo.DisableIdempotentWrite()},
	AcksNone:   {kgo.RequiredAcks(kgo.NoAck()), kgo.DisableIdempotentWrite()},
}

var codecOptions = map[Codec]kgo.CompressionCodec{
	CodecNone: kgo.NoCompression(), CodecLZ4: kgo.Lz4Compression(), CodecZstd: kgo.ZstdCompression(),
	CodecSnappy: kgo.SnappyCompression(), CodecGzip: kgo.GzipCompression(),
}

// Acks returns the options for an acks mode.
func (ClientOptions) Acks(m AcksMode) ([]kgo.Opt, error) {
	o, ok := acksOptions[m]
	if !ok {
		return nil, fmt.Errorf("unknown acks %q (all|leader|none)", m)
	}
	return o, nil
}

// Compression returns the option for a codec.
func (ClientOptions) Compression(c Codec) (kgo.Opt, error) {
	codec, ok := codecOptions[c]
	if !ok {
		names := make([]string, 0, len(codecOptions))
		for k := range codecOptions {
			names = append(names, string(k))
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown compression %q (%v)", c, names)
	}
	return kgo.ProducerBatchCompression(codec), nil
}
