package boundary

// Format names a record format of a route source or destination (route schema `format.type`).
type Format string

const (
	FormatJSON     Format = "json"
	FormatString   Format = "string"
	FormatBytes    Format = "bytes"
	FormatAvro     Format = "avro"
	FormatProtobuf Format = "protobuf"
)

// Registry selects the Marshaller of a route from its source/destination formats.
type Registry struct {
	marshallers map[MarshallerKind]Marshaller
	byFormat    map[Format]MarshallerKind
	fallback    MarshallerKind
}

// NewRegistry returns the standard registry: Avro and Protobuf use tagged values, the rest JSON.
func NewRegistry() *Registry {
	r := &Registry{marshallers: map[MarshallerKind]Marshaller{}, byFormat: map[Format]MarshallerKind{}, fallback: MarshallerJSON}
	r.Register(JSONMarshaller{})
	r.Register(TaggedMarshaller{})
	r.Bind(FormatAvro, MarshallerTagged)
	r.Bind(FormatProtobuf, MarshallerTagged)
	return r
}

// Register adds or replaces a marshaller under its kind.
func (r *Registry) Register(m Marshaller) {
	r.marshallers[m.Kind()] = m
}

// Bind maps a format to a marshaller kind; unbound formats use the fallback.
func (r *Registry) Bind(f Format, kind MarshallerKind) {
	r.byFormat[f] = kind
}

// Get returns the marshaller of kind.
func (r *Registry) Get(kind MarshallerKind) (Marshaller, bool) {
	m, ok := r.marshallers[kind]
	return m, ok
}

// Select returns the first non-fallback marshaller bound to any of formats, else the fallback,
// so a route that reads or writes Avro/Protobuf anywhere keeps schema types across the boundary.
func (r *Registry) Select(formats []Format) Marshaller {
	for _, f := range formats {
		kind, ok := r.byFormat[f]
		if m, registered := r.marshallers[kind]; ok && registered && kind != r.fallback {
			return m
		}
	}
	return r.marshallers[r.fallback]
}
