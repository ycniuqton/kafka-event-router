package metrics

import "time"

// Tone is the severity colour the UI renders for a value.
type Tone string

const (
	ToneOK   Tone = "ok"
	ToneWarn Tone = "warn"
	ToneErr  Tone = "err"
)

// ToneRank orders tones for sorting (higher is more severe).
var ToneRank = map[Tone]int{ToneOK: 0, ToneWarn: 1, ToneErr: 2}

// Thresholds are the server-side lag limits and detection windows (config-overridable).
type Thresholds struct {
	RouteLagWarn    int64         `json:"routeLagWarn"`
	RouteLagError   int64         `json:"routeLagError"`
	TotalLagWarn    int64         `json:"totalLagWarn"`
	TotalLagError   int64         `json:"totalLagError"`
	ZeroMatchWindow time.Duration `json:"-"`
	LagGrowthWindow time.Duration `json:"-"`
}

// DefaultThresholds are the design values (SysDesign §12, UI §4).
var DefaultThresholds = Thresholds{
	RouteLagWarn: 2000, RouteLagError: 20000, TotalLagWarn: 10000, TotalLagError: 40000,
	ZeroMatchWindow: 30 * time.Minute, LagGrowthWindow: 15 * time.Minute,
}

// TotalTone classifies the summed consumer lag.
func (t Thresholds) TotalTone(lag float64) Tone {
	return LagBand{Warn: t.TotalLagWarn, Err: t.TotalLagError}.Tone(lag)
}

// RouteTone classifies one route's consumer lag.
func (t Thresholds) RouteTone(lag float64) Tone {
	return LagBand{Warn: t.RouteLagWarn, Err: t.RouteLagError}.Tone(lag)
}

// LagBand is one warn/error pair of lag limits.
type LagBand struct {
	Warn int64
	Err  int64
}

// Tone returns err above Err, warn above Warn, ok otherwise.
func (b LagBand) Tone(lag float64) Tone {
	if lag > float64(b.Err) {
		return ToneErr
	}
	if lag > float64(b.Warn) {
		return ToneWarn
	}
	return ToneOK
}
