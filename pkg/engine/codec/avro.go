package codec

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/hamba/avro/v2"

	"github.com/eventrouter/router/pkg/config"
)

// ErrSubjectRequired reports an encode without a subject to resolve the schema from.
var ErrSubjectRequired = errors.New("encoding with a schema registry needs a subject")

// ErrSchemaType reports a registry schema of another language than the codec.
var ErrSchemaType = errors.New("registry schema has an unexpected type")

// AvroFactory builds AvroCodecs over the connection registry.
type AvroFactory struct{}

func (AvroFactory) Format() config.FormatType { return config.FormatTypeAvro }

func (AvroFactory) New(spec CodecSpec) (Codec, error) {
	reg := spec.RegistryOrNone()
	c := &AvroCodec{schemas: &AvroSchemas{Registry: reg, parsed: NewLRU[int32, avro.Schema](DefaultSchemaCacheSize)},
		values: DefaultAvroValues(), subjects: SubjectResolver{Registry: reg}}
	if spec.Format.Subject != nil {
		c.subject = *spec.Format.Subject
	}
	return c, nil
}

// AvroCodec decodes Confluent-framed Avro by schema id and encodes with the latest
// schema of its subject.
type AvroCodec struct {
	schemas  *AvroSchemas
	values   *AvroValues
	subjects SubjectResolver
	subject  string
}

func (*AvroCodec) Format() config.FormatType { return config.FormatTypeAvro }

func (c *AvroCodec) Decode(ctx context.Context, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	frame, err := Framing{}.Parse(raw)
	if err != nil {
		return nil, err
	}
	schema, err := c.schemas.ByID(ctx, frame.SchemaID)
	if err != nil {
		return nil, err
	}
	v, err := AvroBody{}.Read(schema, frame.Body)
	if err != nil {
		return nil, err
	}
	return c.values.ToEngine(schema, v), nil
}

func (c *AvroCodec) Encode(ctx context.Context, v any) ([]byte, error) {
	rs, err := c.subjects.Latest(ctx, c.subject)
	if err != nil {
		return nil, err
	}
	schema, err := c.schemas.ByID(ctx, rs.ID)
	if err != nil {
		return nil, err
	}
	av, err := c.values.ToAvro(schema, v)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnencodable, err)
	}
	body, err := avroAPI.Marshal(schema, av)
	if err != nil {
		return nil, fmt.Errorf("%w: avro: %v", ErrUnencodable, err)
	}
	return Framing{}.Write(rs.ID, body), nil
}

// AvroLimit bounds allocations driven by untrusted payload lengths.
type AvroLimit int

const (
	AvroBodyBuffer       AvroLimit = 64
	AvroMaxBytes         AvroLimit = 16 << 20
	AvroMaxSliceElements AvroLimit = 1 << 20
)

// avroAPI is the hamba configuration shared by all Avro codecs.
var avroAPI = avro.Config{MaxByteSliceSize: int(AvroMaxBytes), MaxSliceAllocSize: int(AvroMaxSliceElements)}.Freeze()

// AvroBody decodes one Avro datum and reports truncated input (which avro.Unmarshal
// silently accepts as null).
type AvroBody struct{}

// Read decodes body with schema.
func (AvroBody) Read(schema avro.Schema, body []byte) (any, error) {
	r := avro.NewReader(bytes.NewReader(body), max(len(body), int(AvroBodyBuffer)), avro.WithReaderConfig(avroAPI))
	var v any
	r.ReadVal(schema, &v)
	if r.Error != nil {
		return nil, fmt.Errorf("%w: avro: %v", ErrMalformed, r.Error)
	}
	return v, nil
}

// SubjectResolver returns the latest registered schema of the encode subject.
type SubjectResolver struct {
	Registry SchemaRegistry
}

// Latest fails with ErrSubjectRequired when subject is empty.
func (r SubjectResolver) Latest(ctx context.Context, subject string) (RegisteredSchema, error) {
	if subject == "" {
		return RegisteredSchema{}, ErrSubjectRequired
	}
	return r.Registry.Latest(ctx, subject)
}

// AvroSchemas fetches and parses Avro schemas by id (with references), caching parsed schemas.
type AvroSchemas struct {
	Registry SchemaRegistry
	parsed   *LRU[int32, avro.Schema]
}

// ByID returns the parsed schema of id.
func (s *AvroSchemas) ByID(ctx context.Context, id int32) (avro.Schema, error) {
	if sc, ok := s.parsed.Get(id); ok {
		return sc, nil
	}
	rs, err := s.Registry.SchemaByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rs.EffectiveType() != SchemaTypeAvro {
		return nil, fmt.Errorf("%w: schema %d is %s", ErrSchemaType, id, rs.EffectiveType())
	}
	cache := &avro.SchemaCache{}
	if err := s.references(ctx, rs.References, cache, 0); err != nil {
		return nil, err
	}
	sc, err := avro.ParseWithCache(rs.Text, "", cache)
	if err != nil {
		return nil, fmt.Errorf("%w: avro schema %d: %v", ErrMalformed, id, err)
	}
	s.parsed.Put(id, sc)
	return sc, nil
}

// MaxReferenceDepth bounds nested schema references.
const MaxReferenceDepth = 16

// references parses referenced schemas (depth first) into the named-type cache.
func (s *AvroSchemas) references(ctx context.Context, refs []SchemaReference, cache *avro.SchemaCache, depth int) error {
	if depth > MaxReferenceDepth {
		return fmt.Errorf("%w: schema references nested too deeply", ErrMalformed)
	}
	for _, ref := range refs {
		rs, err := s.Registry.Version(ctx, ref.Subject, ref.Version)
		if err != nil {
			return err
		}
		if err := s.references(ctx, rs.References, cache, depth+1); err != nil {
			return err
		}
		if _, err := avro.ParseWithCache(rs.Text, "", cache); err != nil {
			return fmt.Errorf("%w: avro reference %s: %v", ErrMalformed, ref.Name, err)
		}
	}
	return nil
}
