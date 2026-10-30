package sandbox

// PoolDefault holds pool sizing defaults.
type PoolDefault int

const (
	PoolMaxIdleDefault     PoolDefault = 4
	PoolWarmDefault        PoolDefault = 1
	PoolParallelismDefault PoolDefault = 1
)

// Config is the service-level sandbox configuration (env keys ROUTER_SANDBOX_*).
type Config struct {
	Engine      EngineKind
	Maxima      Limits
	ResetPolicy ResetPolicy
	PoolMaxIdle int
	CacheDir    string
	Programs    ProgramCacheKind
	Autoscale   AutoscaleTuning
}

// DefaultConfig returns the documented defaults of design §4.
func DefaultConfig() Config {
	return Config{
		Engine:      EngineQuickJSWazero,
		Maxima:      LimitMaximum,
		ResetPolicy: ResetFreshContext,
		PoolMaxIdle: int(PoolMaxIdleDefault),
		Programs:    ProgramCacheRoute,
		Autoscale:   AutoscaleDefault,
	}
}

// withDefaults fills autoscaling settings a hand-built Config left empty.
func (c Config) withDefaults() Config {
	if c.Autoscale == (AutoscaleTuning{}) {
		c.Autoscale = AutoscaleDefault
	}
	return c
}
