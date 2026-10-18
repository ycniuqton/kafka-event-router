package compile

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/path"
)

// KeySelector decides the output key per destination key mode.
type KeySelector interface {
	Mode() config.KeyMode
	Key(o *engine.Out) any
}

// HeaderSelector decides which input headers are kept per destination header mode.
type HeaderSelector interface {
	Mode() config.HeaderMode
	Select(hs []engine.Header, keep map[string]bool) []engine.Header
}

// headerSelectors is the registry of header modes.
var headerSelectors = map[config.HeaderMode]HeaderSelector{
	config.HeaderModeKeepAll:      keepAllHeaders{},
	config.HeaderModeKeepSelected: keepSelectedHeaders{},
	config.HeaderModeDropAll:      dropAllHeaders{},
}

// OutputShaper turns an output event into a destination record.
type OutputShaper struct {
	topic    string
	key      KeySelector
	headers  HeaderSelector
	keep     map[string]bool
	add      []engine.Header
	keyCodec codec.Codec
	valCodec codec.Codec
}

// OutputShaperBuilder compiles a destination.
type OutputShaperBuilder struct{}

// Build resolves destination codecs (same_as_source reuses the source format) on the
// destination connection registry, and the key and header policy.
func (OutputShaperBuilder) Build(scope *compilectx.Scope, m *codec.FactoryManager, ptr config.Pointer, d config.Destination, src SourceCodecs) *OutputShaper {
	s := &OutputShaper{topic: d.Topic, keep: map[string]bool{}}
	b := CodecBuilder{Connection: d.ConnectionID}
	valueSpec := src.ValueSpec
	if d.Format != nil && d.Format.Type != config.DestinationFormatTypeSameAsSource {
		valueSpec = config.Format{Type: config.FormatType(d.Format.Type), Subject: d.Format.Subject, MessageType: d.Format.MessageType}
	}
	s.valCodec = b.New(scope, m, SubjectDefaults{}.For(valueSpec, d.Topic, SubjectSuffixValue), ptr.Token(config.TokenFormat))
	s.keyCodec = b.New(scope, m, SubjectDefaults{}.For(src.KeySpec, d.Topic, SubjectSuffixKey), ptr.Token(config.TokenKey))
	s.key = KeySelectorBuilder{}.Build(scope, ptr.Token(config.TokenKey), d.Key)
	s.headers = headerSelectors[config.HeaderModeKeepAll]
	if d.Headers != nil {
		s.headers = headerSelectors[d.Headers.Mode]
		for _, k := range d.Headers.Keep {
			s.keep[k] = true
		}
		for _, h := range d.Headers.Add {
			s.add = append(s.add, engine.Header{Key: h.Key, Value: []byte(h.Value)})
		}
	}
	return s
}

// KeySelectorBuilder compiles a destination key policy.
type KeySelectorBuilder struct{}

// Build returns the selector for the policy (absent = keep).
func (KeySelectorBuilder) Build(scope *compilectx.Scope, ptr config.Pointer, kp *config.KeyPolicy) KeySelector {
	if kp == nil {
		return keepKey{}
	}
	selectors := map[config.KeyMode]func() KeySelector{
		config.KeyModeKeep: func() KeySelector { return keepKey{} },
		config.KeyModeNone: func() KeySelector { return noKey{} },
		config.KeyModeField: func() KeySelector {
			if kp.Field == nil {
				scope.Fail(config.IssueSchemaInvalid, ptr, "key mode field requires field")
				return noKey{}
			}
			p, _ := scope.Path(*kp.Field, ptr.Token(config.TokenField))
			return fieldKey{path: p}
		},
	}
	return selectors[kp.Mode]()
}

type keepKey struct{}

func (keepKey) Mode() config.KeyMode  { return config.KeyModeKeep }
func (keepKey) Key(o *engine.Out) any { return o.Key }

type noKey struct{}

func (noKey) Mode() config.KeyMode { return config.KeyModeNone }
func (noKey) Key(*engine.Out) any  { return nil }

type fieldKey struct {
	path path.FieldPath
}

func (fieldKey) Mode() config.KeyMode { return config.KeyModeField }
func (k fieldKey) Key(o *engine.Out) any {
	v, _ := k.path.Get(&o.Event)
	return v
}

type keepAllHeaders struct{}

func (keepAllHeaders) Mode() config.HeaderMode { return config.HeaderModeKeepAll }
func (keepAllHeaders) Select(hs []engine.Header, _ map[string]bool) []engine.Header {
	return append([]engine.Header{}, hs...)
}

type keepSelectedHeaders struct{}

func (keepSelectedHeaders) Mode() config.HeaderMode { return config.HeaderModeKeepSelected }
func (keepSelectedHeaders) Select(hs []engine.Header, keep map[string]bool) []engine.Header {
	out := []engine.Header{}
	for _, h := range hs {
		if keep[h.Key] {
			out = append(out, h)
		}
	}
	return out
}

type dropAllHeaders struct{}

func (dropAllHeaders) Mode() config.HeaderMode { return config.HeaderModeDropAll }
func (dropAllHeaders) Select([]engine.Header, map[string]bool) []engine.Header {
	return []engine.Header{}
}
