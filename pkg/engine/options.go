package engine

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
)

// CompileOptions are the ports and route identity a compile needs; nil Scripts makes
// js steps and expr nodes compile errors, nil Library makes library steps compile errors,
// nil SchemaRegistries makes avro/protobuf records fail at runtime (no registry); an empty
// Codec selects DefaultCodecMode for RecordRunner.Run.
type CompileOptions struct {
	Scripts          script.Runtime
	Library          config.LibraryResolver
	Helpers          ctxhelpers.Set
	Meta             RouteMeta
	SchemaRegistries codec.RegistryProvider
	Codec            CodecMode
}
