package config

// EnvEngineCodec selects the worker's JSON record codec: fast (default) or tree.
const EnvEngineCodec EnvKey = "ROUTER_ENGINE_CODEC"

// EngineCodec returns the configured codec name; empty means the engine default.
func (l RuntimeLoader) EngineCodec() string {
	return l.str(EnvEngineCodec, "")
}
