package codec

import (
	"github.com/hamba/avro/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// NewAvroSchemas returns a cached Avro schema loader over reg (used outside codecs, e.g. preview schema merge).
func NewAvroSchemas(reg SchemaRegistry) *AvroSchemas {
	return &AvroSchemas{Registry: reg, parsed: NewLRU[int32, avro.Schema](DefaultSchemaCacheSize)}
}

// NewProtoFiles returns a cached protobuf file loader over reg (used outside codecs, e.g. preview schema merge).
func NewProtoFiles(reg SchemaRegistry) *ProtoFiles {
	return &ProtoFiles{Registry: reg, compiled: NewLRU[int32, protoreflect.FileDescriptor](DefaultSchemaCacheSize)}
}
