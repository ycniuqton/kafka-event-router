package compile

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compilectx"
)

// SourceCodecs are the key and value codecs of the source topic with their specs.
type SourceCodecs struct {
	Key       codec.Codec
	Value     codec.Codec
	KeySpec   config.Format
	ValueSpec config.Format
}

// DefaultKeyFormat is used when source.keyFormat is absent.
const DefaultKeyFormat = config.FormatTypeString

// SubjectSuffix is the TopicNameStrategy suffix of registry subjects.
type SubjectSuffix string

const (
	SubjectSuffixKey   SubjectSuffix = "-key"
	SubjectSuffixValue SubjectSuffix = "-value"
)

// Build creates both codecs, recording FormatUnsupported issues.
func (SourceCodecs) Build(scope *compilectx.Scope, m *codec.FactoryManager, src config.Source) SourceCodecs {
	ptr := config.PointerRoot.Token(config.TokenSource)
	keySpec := config.Format{Type: DefaultKeyFormat}
	if src.KeyFormat != nil {
		keySpec = *src.KeyFormat
	}
	b := CodecBuilder{Connection: src.ConnectionID}
	return SourceCodecs{
		Key:       b.New(scope, m, keySpec, ptr.Token(config.TokenKeyFormat)),
		Value:     b.New(scope, m, src.Format, ptr.Token(config.TokenFormat)),
		KeySpec:   keySpec,
		ValueSpec: src.Format,
	}
}

// CodecBuilder creates codecs bound to the schema registry of one connection.
type CodecBuilder struct {
	Connection string
}

// New returns the codec or nil after recording an issue at ptr.
func (b CodecBuilder) New(scope *compilectx.Scope, m *codec.FactoryManager, spec config.Format, ptr config.Pointer) codec.Codec {
	reg := codec.ConnectionRegistry{Provider: scope.Registries, ConnectionID: b.Connection}
	c, err := m.NewSpec(codec.CodecSpec{Format: spec, Registry: reg})
	if err != nil {
		scope.Fail(config.IssueFormatUnsupported, ptr.Token(config.TokenType), "%v", err)
		return nil
	}
	return c
}

// SubjectDefaults applies the TopicNameStrategy (`<topic>-key` / `<topic>-value`) to
// formats without a subject, so registry-backed destinations can encode.
type SubjectDefaults struct{}

// For returns spec with its subject defaulted for topic.
func (SubjectDefaults) For(spec config.Format, topic string, suffix SubjectSuffix) config.Format {
	if spec.Subject == nil {
		s := topic + string(suffix)
		spec.Subject = &s
	}
	return spec
}
