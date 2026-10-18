package kafka

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// PropertyKey is one allow-listed advanced client property.
type PropertyKey string

const (
	PropClientID             PropertyKey = "client.id"
	PropRequestTimeoutMs     PropertyKey = "request.timeout.ms"
	PropMetadataMaxAgeMs     PropertyKey = "metadata.max.age.ms"
	PropConnectionsMaxIdleMs PropertyKey = "connections.max.idle.ms"
	PropFetchMaxBytes        PropertyKey = "fetch.max.bytes"
	PropSocketSetupTimeoutMs PropertyKey = "socket.connection.setup.timeout.ms"
)

// PropertyMessage holds the user-facing texts of property validation.
type PropertyMessage string

const (
	MsgPropertyUnknown  PropertyMessage = "This property is not supported. Allowed: %s."
	MsgPropertyNotInt   PropertyMessage = "Enter a positive whole number."
	MsgPropertyNotEmpty PropertyMessage = "Enter a value."
)

// PropertyMapper validates one property value and maps it to a franz-go option.
type PropertyMapper interface {
	Key() PropertyKey
	Map(value string) (kgo.Opt, error)
}

// StringPropertyMapper maps a free-text property (e.g. client.id).
type StringPropertyMapper struct {
	key PropertyKey
	opt func(string) kgo.Opt
}

// Key implements PropertyMapper.
func (m StringPropertyMapper) Key() PropertyKey { return m.key }

// Map implements PropertyMapper.
func (m StringPropertyMapper) Map(value string) (kgo.Opt, error) {
	if value == "" {
		return nil, fmt.Errorf("%s", MsgPropertyNotEmpty)
	}
	return m.opt(value), nil
}

// MillisPropertyMapper maps a positive millisecond property to a duration option.
type MillisPropertyMapper struct {
	key PropertyKey
	opt func(time.Duration) kgo.Opt
}

// Key implements PropertyMapper.
func (m MillisPropertyMapper) Key() PropertyKey { return m.key }

// Map implements PropertyMapper.
func (m MillisPropertyMapper) Map(value string) (kgo.Opt, error) {
	n, err := PositiveInt{}.Parse(value)
	if err != nil {
		return nil, err
	}
	return m.opt(time.Duration(n) * time.Millisecond), nil
}

// BytesPropertyMapper maps a positive byte-count property.
type BytesPropertyMapper struct {
	key PropertyKey
	opt func(int32) kgo.Opt
}

// Key implements PropertyMapper.
func (m BytesPropertyMapper) Key() PropertyKey { return m.key }

// Map implements PropertyMapper.
func (m BytesPropertyMapper) Map(value string) (kgo.Opt, error) {
	n, err := PositiveInt{}.Parse(value)
	if err != nil {
		return nil, err
	}
	return m.opt(int32(n)), nil
}

// PositiveInt parses positive 32-bit integers.
type PositiveInt struct{}

// Parse returns the value or a user-facing error.
func (PositiveInt) Parse(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s", MsgPropertyNotInt)
	}
	return n, nil
}

// PropertyCatalog is the Manager of the advanced-property allow-list.
type PropertyCatalog struct {
	mappers map[PropertyKey]PropertyMapper
}

// NewPropertyCatalog returns an empty catalog.
func NewPropertyCatalog() *PropertyCatalog {
	return &PropertyCatalog{mappers: map[PropertyKey]PropertyMapper{}}
}

// DefaultPropertyCatalog registers the documented allow-list.
func DefaultPropertyCatalog() *PropertyCatalog {
	c := NewPropertyCatalog()
	c.Register(StringPropertyMapper{key: PropClientID, opt: kgo.ClientID})
	c.Register(MillisPropertyMapper{key: PropRequestTimeoutMs, opt: kgo.RequestTimeoutOverhead})
	c.Register(MillisPropertyMapper{key: PropMetadataMaxAgeMs, opt: kgo.MetadataMaxAge})
	c.Register(MillisPropertyMapper{key: PropConnectionsMaxIdleMs, opt: kgo.ConnIdleTimeout})
	c.Register(MillisPropertyMapper{key: PropSocketSetupTimeoutMs, opt: kgo.DialTimeout})
	c.Register(BytesPropertyMapper{key: PropFetchMaxBytes, opt: func(b int32) kgo.Opt { return kgo.FetchMaxBytes(b) }})
	return c
}

// Register adds a mapper.
func (c *PropertyCatalog) Register(m PropertyMapper) {
	c.mappers[m.Key()] = m
}

// Keys lists the allowed keys in stable order.
func (c *PropertyCatalog) Keys() []PropertyKey {
	out := make([]PropertyKey, 0, len(c.mappers))
	for k := range c.mappers {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Validate returns one field error per unknown key or invalid value (stable order).
func (c *PropertyCatalog) Validate(props map[PropertyKey]string) []FieldError {
	var errs []FieldError
	for _, key := range c.sortedKeys(props) {
		if _, err := c.option(key, props[key]); err != nil {
			errs = append(errs, *err)
		}
	}
	return errs
}

// Options maps every property to its franz-go option.
func (c *PropertyCatalog) Options(props map[PropertyKey]string) ([]kgo.Opt, error) {
	opts := make([]kgo.Opt, 0, len(props))
	for _, key := range c.sortedKeys(props) {
		opt, ferr := c.option(key, props[key])
		if ferr != nil {
			return nil, fmt.Errorf("kafka: property %s: %s", key, ferr.Message)
		}
		opts = append(opts, opt)
	}
	return opts, nil
}

func (c *PropertyCatalog) option(key PropertyKey, value string) (kgo.Opt, *FieldError) {
	m, ok := c.mappers[key]
	if !ok {
		msg := fmt.Sprintf(string(MsgPropertyUnknown), c.allowList())
		return nil, &FieldError{Path: PropertyPath(string(key)), Code: CodeUnknown, Message: msg}
	}
	opt, err := m.Map(value)
	if err != nil {
		return nil, &FieldError{Path: PropertyPath(string(key)), Code: CodeInvalid, Message: err.Error()}
	}
	return opt, nil
}

func (c *PropertyCatalog) allowList() string {
	keys := c.Keys()
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += string(k)
	}
	return out
}

func (c *PropertyCatalog) sortedKeys(props map[PropertyKey]string) []PropertyKey {
	keys := make([]PropertyKey, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
