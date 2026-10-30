package membudget

// FetchBytes bounds the fetch buffering of one source consumer: fetched but not yet polled data is outside
// the budget, so each consumer may buffer only a small slice of it (franz-go's own default is 50 MiB).
type FetchBytes int64

const (
	FetchFloorBytes     FetchBytes = 1 << 20
	FetchCeilingBytes   FetchBytes = 50 << 20
	FetchPartitionBytes FetchBytes = 1 << 20
	FetchBudgetPercent  Watermark  = 1
)

// FetchLimits are the franz-go fetch caps of one source consumer.
type FetchLimits struct {
	MaxBytes          int32
	PartitionMaxBytes int32
}

// FetchLimits sizes one consumer's fetches at 1 % of the budget within [1 MiB, 50 MiB] (unlimited = 50 MiB).
func (b *Budget) FetchLimits() FetchLimits {
	n := min(max(FetchBytes(FetchBudgetPercent.of(b.limit)), FetchFloorBytes), FetchCeilingBytes)
	return FetchLimits{MaxBytes: int32(n), PartitionMaxBytes: int32(min(n, FetchPartitionBytes))}
}
