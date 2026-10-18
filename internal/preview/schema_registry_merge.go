package preview

import (
	"context"

	"github.com/hamba/avro/v2"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// RegistryField is one field declared by a registry schema, as path segments from the event root.
type RegistryField struct {
	Segments []string
	Type     JSONType
}

// RegistryFieldSource lists the fields of the registry schema a framed payload was written with.
type RegistryFieldSource interface {
	Format() config.FormatType
	Fields(ctx context.Context, reg codec.SchemaRegistry, raw []byte) ([]RegistryField, error)
}

// RegistryFieldSources selects the field source by value format; formats without one are not merged.
type RegistryFieldSources struct {
	sources map[config.FormatType]RegistryFieldSource
}

// NewRegistryFieldSources registers sources.
func NewRegistryFieldSources(sources ...RegistryFieldSource) *RegistryFieldSources {
	r := &RegistryFieldSources{sources: map[config.FormatType]RegistryFieldSource{}}
	for _, s := range sources {
		r.sources[s.Format()] = s
	}
	return r
}

// DefaultRegistryFieldSources registers avro and protobuf.
func DefaultRegistryFieldSources() *RegistryFieldSources {
	return NewRegistryFieldSources(AvroFieldSource{}, ProtobufFieldSource{})
}

// RegistrySchemaMerger adds the registry schema's fields to an inferred schema (FR-8): fields seen in
// samples are flagged registry=true, fields never seen are added with frequency 0.
type RegistrySchemaMerger struct {
	Sources *RegistryFieldSources
	Decoder SampleDecoder
}

// Merge returns fs extended with the registry fields of the first framed record; failures keep fs as is.
func (m RegistrySchemaMerger) Merge(ctx context.Context, recs []SampleRecord, f SampleFormat, fs FieldSchema) FieldSchema {
	if m.Sources == nil {
		return fs
	}
	src, ok := m.Sources.sources[f.Type]
	raw, framed := FramedPayloads{}.First(recs)
	if !ok || !framed {
		return fs
	}
	fields, err := src.Fields(ctx, m.Decoder.registry(f), raw)
	if err != nil {
		return fs
	}
	tree := SchemaTreeFor(&fs.Root)
	for _, field := range fields {
		tree.Ensure(field)
	}
	return fs
}

// FramedPayloads finds schema-registry framed values.
type FramedPayloads struct{}

// First returns the first record value carrying Confluent framing.
func (FramedPayloads) First(recs []SampleRecord) ([]byte, bool) {
	for _, r := range recs {
		if _, err := (codec.Framing{}).Parse(r.Value); err == nil {
			return r.Value, true
		}
	}
	return nil, false
}

// SchemaTree inserts registry fields into an inferred FieldNode tree using the inferrer's path syntax.
type SchemaTree struct {
	root *FieldNode
}

// SchemaTreeFor edits the tree under root in place.
func SchemaTreeFor(root *FieldNode) SchemaTree { return SchemaTree{root: root} }

// Ensure walks (or creates) the nodes of field and flags the leaf as declared by the registry.
func (t SchemaTree) Ensure(field RegistryField) {
	node, path := t.root, ""
	for _, seg := range field.Segments {
		path = SchemaPaths{}.Join(path, seg)
		node = SchemaTree{}.child(node, path)
	}
	node.Registry = true
	if _, seen := node.Types[field.Type]; !seen {
		node.Types[field.Type] = 0
	}
}

func (SchemaTree) child(n *FieldNode, path string) *FieldNode {
	for i := range n.Children {
		if n.Children[i].Path == path {
			return &n.Children[i]
		}
	}
	n.Children = append(n.Children, FieldNode{Path: path, Types: map[JSONType]int{}, TopValues: []ValueCount{}, Children: []FieldNode{}})
	return &n.Children[len(n.Children)-1]
}

// SchemaPaths joins path segments like the inferrer ("value.items[].sku").
type SchemaPaths struct{}

// Join appends seg to path.
func (SchemaPaths) Join(path, seg string) string {
	switch {
	case path == "":
		return seg
	case seg == string(PathArrayItem):
		return path + seg
	}
	return path + string(PathSeparator) + seg
}

// FieldList collects registry fields while a schema is walked.
type FieldList struct {
	fields []RegistryField
}

// Add records a field at segs (copied).
func (l *FieldList) Add(segs []string, t JSONType) {
	l.fields = append(l.fields, RegistryField{Segments: append([]string(nil), segs...), Type: t})
}

// Extend returns segs + seg without aliasing segs.
func (FieldList) Extend(segs []string, seg string) []string {
	return append(append([]string(nil), segs...), seg)
}

// valueRoot is where value fields live in the event document.
var valueRoot = []string{string(engine.FieldValue)}

// AvroFieldSource reads fields of the Avro writer schema (by framed schema id).
type AvroFieldSource struct{}

func (AvroFieldSource) Format() config.FormatType { return config.FormatTypeAvro }

// Fields implements RegistryFieldSource.
func (AvroFieldSource) Fields(ctx context.Context, reg codec.SchemaRegistry, raw []byte) ([]RegistryField, error) {
	frame, err := codec.Framing{}.Parse(raw)
	if err != nil {
		return nil, err
	}
	schema, err := codec.NewAvroSchemas(reg).ByID(ctx, frame.SchemaID)
	if err != nil {
		return nil, err
	}
	w := &AvroFieldWalker{}
	w.Walk(schema, valueRoot, 0)
	return w.list.fields, nil
}

// avroJSONTypes maps Avro types to the JSON type the engine decodes them to.
var avroJSONTypes = map[avro.Type]JSONType{
	avro.String: TypeString, avro.Bytes: TypeString, avro.Enum: TypeString, avro.Fixed: TypeString,
	avro.Int: TypeNumber, avro.Long: TypeNumber, avro.Float: TypeNumber, avro.Double: TypeNumber,
	avro.Boolean: TypeBoolean, avro.Null: TypeNull, avro.Record: TypeObject, avro.Map: TypeObject, avro.Array: TypeArray,
}

// avroLogicalTypes maps logical types the engine renders as text.
var avroLogicalTypes = map[avro.LogicalType]JSONType{
	avro.Decimal: TypeString, avro.UUID: TypeString, avro.Date: TypeString, avro.TimestampMillis: TypeString,
	avro.TimestampMicros: TypeString, avro.LocalTimestampMillis: TypeString, avro.LocalTimestampMicros: TypeString,
}

// AvroFieldWalker walks an Avro schema into registry fields (records, arrays, unions, refs).
type AvroFieldWalker struct {
	list FieldList
}

// Walk records s at segs and descends into its members.
func (w *AvroFieldWalker) Walk(s avro.Schema, segs []string, depth int) {
	if depth > int(SchemaMaxDepth) {
		return
	}
	switch t := s.(type) {
	case *avro.RefSchema:
		w.Walk(t.Schema(), segs, depth+1)
	case *avro.UnionSchema:
		for _, sub := range t.Types() {
			w.union(sub, segs, depth)
		}
	case *avro.RecordSchema:
		w.list.Add(segs, TypeObject)
		for _, f := range t.Fields() {
			w.Walk(f.Type(), FieldList{}.Extend(segs, f.Name()), depth+1)
		}
	case *avro.ArraySchema:
		w.list.Add(segs, TypeArray)
		w.Walk(t.Items(), FieldList{}.Extend(segs, string(PathArrayItem)), depth+1)
	default:
		w.list.Add(segs, w.leafType(s))
	}
}

func (w *AvroFieldWalker) union(sub avro.Schema, segs []string, depth int) {
	if sub.Type() != avro.Null {
		w.Walk(sub, segs, depth+1)
	}
}

func (AvroFieldWalker) leafType(s avro.Schema) JSONType {
	if ls, ok := s.(avro.LogicalTypeSchema); ok && ls.Logical() != nil {
		if t, known := avroLogicalTypes[ls.Logical().Type()]; known {
			return t
		}
	}
	if t, ok := avroJSONTypes[s.Type()]; ok {
		return t
	}
	return TypeString
}

// ProtobufFieldSource reads fields of the protobuf message a framed payload names (schema id + indexes).
type ProtobufFieldSource struct{}

func (ProtobufFieldSource) Format() config.FormatType { return config.FormatTypeProtobuf }

// Fields implements RegistryFieldSource.
func (ProtobufFieldSource) Fields(ctx context.Context, reg codec.SchemaRegistry, raw []byte) ([]RegistryField, error) {
	frame, err := codec.Framing{}.Parse(raw)
	if err != nil {
		return nil, err
	}
	fd, err := codec.NewProtoFiles(reg).ByID(ctx, frame.SchemaID)
	if err != nil {
		return nil, err
	}
	indexes, _, err := codec.MessageIndexes{}.Read(frame.Body)
	if err != nil {
		return nil, err
	}
	md, err := codec.ProtoMessages{}.ByIndexes(fd, indexes)
	if err != nil {
		return nil, err
	}
	w := &ProtoFieldWalker{}
	w.Message(md, valueRoot, 0)
	return w.list.fields, nil
}

// protoJSONTypes maps protobuf scalar kinds to the JSON type the engine decodes them to.
var protoJSONTypes = map[protoreflect.Kind]JSONType{
	protoreflect.BoolKind: TypeBoolean, protoreflect.EnumKind: TypeString, protoreflect.StringKind: TypeString,
	protoreflect.BytesKind: TypeString, protoreflect.FloatKind: TypeNumber, protoreflect.DoubleKind: TypeNumber,
	protoreflect.Int32Kind: TypeNumber, protoreflect.Sint32Kind: TypeNumber, protoreflect.Sfixed32Kind: TypeNumber,
	protoreflect.Int64Kind: TypeNumber, protoreflect.Sint64Kind: TypeNumber, protoreflect.Sfixed64Kind: TypeNumber,
	protoreflect.Uint32Kind: TypeNumber, protoreflect.Fixed32Kind: TypeNumber,
	protoreflect.Uint64Kind: TypeNumber, protoreflect.Fixed64Kind: TypeNumber,
}

// ProtoWellKnownText names messages the engine renders as text.
type ProtoWellKnownText string

const ProtoTimestampMessage ProtoWellKnownText = "google.protobuf.Timestamp"

// ProtoFieldWalker walks a message descriptor into registry fields (nested messages, repeated, maps).
type ProtoFieldWalker struct {
	list FieldList
}

// Message records md at segs and descends into its fields.
func (w *ProtoFieldWalker) Message(md protoreflect.MessageDescriptor, segs []string, depth int) {
	if string(md.FullName()) == string(ProtoTimestampMessage) {
		w.list.Add(segs, TypeString)
		return
	}
	w.list.Add(segs, TypeObject)
	if depth > int(SchemaMaxDepth) {
		return
	}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		w.Field(fields.Get(i), FieldList{}.Extend(segs, string(fields.Get(i).Name())), depth+1)
	}
}

// Field records one field: repeated as arrays, maps as objects, messages recursively.
func (w *ProtoFieldWalker) Field(fd protoreflect.FieldDescriptor, segs []string, depth int) {
	switch {
	case fd.IsMap():
		w.list.Add(segs, TypeObject)
	case fd.IsList():
		w.list.Add(segs, TypeArray)
		w.single(fd, FieldList{}.Extend(segs, string(PathArrayItem)), depth)
	default:
		w.single(fd, segs, depth)
	}
}

func (w *ProtoFieldWalker) single(fd protoreflect.FieldDescriptor, segs []string, depth int) {
	if fd.Message() != nil {
		w.Message(fd.Message(), segs, depth)
		return
	}
	w.list.Add(segs, protoJSONTypes[fd.Kind()])
}
