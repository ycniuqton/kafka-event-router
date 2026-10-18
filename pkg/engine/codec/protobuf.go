package codec

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/eventrouter/router/pkg/config"
)

// ProtoMainFile is the virtual file name of the schema being compiled.
const ProtoMainFile = "__schema__.proto"

// MaxMessageIndexes bounds the message index path read from a frame.
const MaxMessageIndexes = 64

// ProtobufFactory builds ProtobufCodecs over the connection registry.
type ProtobufFactory struct{}

func (ProtobufFactory) Format() config.FormatType { return config.FormatTypeProtobuf }

func (ProtobufFactory) New(spec CodecSpec) (Codec, error) {
	reg := spec.RegistryOrNone()
	c := &ProtobufCodec{files: &ProtoFiles{Registry: reg, compiled: NewLRU[int32, protoreflect.FileDescriptor](DefaultSchemaCacheSize)},
		values: DefaultProtoValues(), subjects: SubjectResolver{Registry: reg}}
	if spec.Format.Subject != nil {
		c.subject = *spec.Format.Subject
	}
	if spec.Format.MessageType != nil {
		c.messageType = *spec.Format.MessageType
	}
	return c, nil
}

// ProtobufCodec decodes Confluent-framed protobuf (schema id + message indexes) into
// engine maps and encodes maps with the latest schema of its subject.
type ProtobufCodec struct {
	files       *ProtoFiles
	values      *ProtoValues
	subjects    SubjectResolver
	subject     string
	messageType string
}

func (*ProtobufCodec) Format() config.FormatType { return config.FormatTypeProtobuf }

func (c *ProtobufCodec) Decode(ctx context.Context, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	frame, err := Framing{}.Parse(raw)
	if err != nil {
		return nil, err
	}
	fd, err := c.files.ByID(ctx, frame.SchemaID)
	if err != nil {
		return nil, err
	}
	indexes, body, err := MessageIndexes{}.Read(frame.Body)
	if err != nil {
		return nil, err
	}
	md, err := ProtoMessages{}.ByIndexes(fd, indexes)
	if err != nil {
		return nil, err
	}
	msg, err := ProtoBody{}.Read(md, body)
	if err != nil {
		return nil, err
	}
	return c.values.ToEngine(msg), nil
}

func (c *ProtobufCodec) Encode(ctx context.Context, v any) ([]byte, error) {
	rs, err := c.subjects.Latest(ctx, c.subject)
	if err != nil {
		return nil, err
	}
	fd, err := c.files.ByID(ctx, rs.ID)
	if err != nil {
		return nil, err
	}
	md, indexes, err := ProtoMessages{}.ByName(fd, c.messageType)
	if err != nil {
		return nil, err
	}
	msg := dynamicpb.NewMessage(md)
	if err := c.values.ToProto(msg, v); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnencodable, err)
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("%w: protobuf: %v", ErrUnencodable, err)
	}
	return Framing{}.Write(rs.ID, append(MessageIndexes{}.Write(indexes), body...)), nil
}

// ProtoBody unmarshals a dynamic message; panics of the protobuf runtime on hostile
// input (seen with malformed map entries) become malformed errors.
type ProtoBody struct{}

// Read decodes body as md.
func (ProtoBody) Read(md protoreflect.MessageDescriptor, body []byte) (msg *dynamicpb.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, err = nil, fmt.Errorf("%w: protobuf: %v", ErrMalformed, r)
		}
	}()
	msg = dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil, fmt.Errorf("%w: protobuf: %v", ErrMalformed, err)
	}
	return msg, nil
}

// MessageIndexes reads and writes the Confluent message index path (zig-zag varints;
// a single 0 byte stands for [0]).
type MessageIndexes struct{}

// Read returns the index path and the remaining message bytes.
func (MessageIndexes) Read(b []byte) ([]int, []byte, error) {
	n, size := binary.Varint(b)
	if size <= 0 || n < 0 || n > MaxMessageIndexes {
		return nil, nil, fmt.Errorf("%w: invalid protobuf message indexes", ErrMalformed)
	}
	b = b[size:]
	if n == 0 {
		return []int{0}, b, nil
	}
	out := make([]int, 0, n)
	for i := int64(0); i < n; i++ {
		idx, size := binary.Varint(b)
		if size <= 0 || idx < 0 {
			return nil, nil, fmt.Errorf("%w: invalid protobuf message index", ErrMalformed)
		}
		out = append(out, int(idx))
		b = b[size:]
	}
	return out, b, nil
}

// Write encodes an index path.
func (MessageIndexes) Write(indexes []int) []byte {
	if len(indexes) == 1 && indexes[0] == 0 {
		return []byte{0}
	}
	out := binary.AppendVarint(nil, int64(len(indexes)))
	for _, i := range indexes {
		out = binary.AppendVarint(out, int64(i))
	}
	return out
}

// ProtoMessages finds message descriptors inside a file.
type ProtoMessages struct{}

// ByIndexes walks top-level then nested messages.
func (ProtoMessages) ByIndexes(fd protoreflect.FileDescriptor, indexes []int) (protoreflect.MessageDescriptor, error) {
	msgs := fd.Messages()
	var md protoreflect.MessageDescriptor
	for _, i := range indexes {
		if i >= msgs.Len() {
			return nil, fmt.Errorf("%w: message index %v out of range", ErrMalformed, indexes)
		}
		md = msgs.Get(i)
		msgs = md.Messages()
	}
	if md == nil {
		return nil, fmt.Errorf("%w: schema has no message", ErrMalformed)
	}
	return md, nil
}

// ByName finds a message by full or simple name (empty = first message) and its index path.
func (p ProtoMessages) ByName(fd protoreflect.FileDescriptor, name string) (protoreflect.MessageDescriptor, []int, error) {
	if name == "" {
		md, err := p.ByIndexes(fd, []int{0})
		return md, []int{0}, err
	}
	if md, path, ok := p.find(fd.Messages(), name, nil); ok {
		return md, path, nil
	}
	return nil, nil, fmt.Errorf("%w: message type %q not in schema", ErrUnencodable, name)
}

func (p ProtoMessages) find(msgs protoreflect.MessageDescriptors, name string, prefix []int) (protoreflect.MessageDescriptor, []int, bool) {
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		path := append(append([]int(nil), prefix...), i)
		if string(md.FullName()) == strings.TrimPrefix(name, ".") || string(md.Name()) == name {
			return md, path, true
		}
		if found, fp, ok := p.find(md.Messages(), name, path); ok {
			return found, fp, true
		}
	}
	return nil, nil, false
}

// ProtoFiles fetches registry schemas (with references as imports) and compiles them once per id.
type ProtoFiles struct {
	Registry SchemaRegistry
	compiled *LRU[int32, protoreflect.FileDescriptor]
}

// ByID returns the compiled file of schema id.
func (f *ProtoFiles) ByID(ctx context.Context, id int32) (protoreflect.FileDescriptor, error) {
	if fd, ok := f.compiled.Get(id); ok {
		return fd, nil
	}
	rs, err := f.Registry.SchemaByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rs.EffectiveType() != SchemaTypeProtobuf {
		return nil, fmt.Errorf("%w: schema %d is %s", ErrSchemaType, id, rs.EffectiveType())
	}
	sources := map[string]string{ProtoMainFile: rs.Text}
	if err := f.imports(ctx, rs.References, sources, 0); err != nil {
		return nil, err
	}
	fd, err := f.compile(ctx, sources)
	if err != nil {
		return nil, fmt.Errorf("%w: protobuf schema %d: %v", ErrMalformed, id, err)
	}
	f.compiled.Put(id, fd)
	return fd, nil
}

func (f *ProtoFiles) imports(ctx context.Context, refs []SchemaReference, sources map[string]string, depth int) error {
	if depth > MaxReferenceDepth {
		return fmt.Errorf("%w: schema references nested too deeply", ErrMalformed)
	}
	for _, ref := range refs {
		rs, err := f.Registry.Version(ctx, ref.Subject, ref.Version)
		if err != nil {
			return err
		}
		sources[ref.Name] = rs.Text
		if err := f.imports(ctx, rs.References, sources, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (f *ProtoFiles) compile(ctx context.Context, sources map[string]string) (protoreflect.FileDescriptor, error) {
	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(sources)})
	files, err := (&protocompile.Compiler{Resolver: resolver}).Compile(ctx, ProtoMainFile)
	if err != nil {
		return nil, err
	}
	return files[0], nil
}
