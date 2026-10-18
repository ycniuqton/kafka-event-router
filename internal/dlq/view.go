package dlq

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
)

// Encoding tells how a key, value or header is rendered.
type Encoding string

const (
	EncodingJSON     Encoding = "json"
	EncodingUTF8     Encoding = "utf8"
	EncodingBase64   Encoding = "base64"
	EncodingAvro     Encoding = "avro"
	EncodingProtobuf Encoding = "protobuf"
)

// EncodedValue is a rendered byte value; Data is JSON, a string, base64 text or nil.
type EncodedValue struct {
	Encoding Encoding `json:"encoding"`
	Data     any      `json:"data"`
	SchemaID *int32   `json:"schemaId,omitempty"`
}

// EncodedHeader is a rendered header.
type EncodedHeader struct {
	Key      string   `json:"key"`
	Value    string   `json:"value"`
	Encoding Encoding `json:"encoding"`
}

// SourceView is the source position in responses.
type SourceView struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

// ResolutionView is the resolution in responses.
type ResolutionView struct {
	State       ResolutionState `json:"state"`
	By          string          `json:"by"`
	At          time.Time       `json:"at"`
	Destination *Destination    `json:"destination,omitempty"`
	JobID       string          `json:"jobId,omitempty"`
}

// ItemView is a list row (no payload).
type ItemView struct {
	ID         string              `json:"id"`
	RouteID    string              `json:"routeId"`
	RouteName  string              `json:"routeName"`
	FailedAt   time.Time           `json:"failedAt"`
	ErrorType  record.ErrorType    `json:"errorType"`
	Message    string              `json:"message"`
	Step       string              `json:"step"`
	StepLabel  string              `json:"stepLabel"`
	Branch     string              `json:"branch"`
	VersionNo  int                 `json:"versionNo"`
	Source     SourceView          `json:"source"`
	Resolution *ResolutionView     `json:"resolution"`
	Delivery   config.DeliveryMode `json:"deliveryMode,omitempty"`
}

// DetailView is the detail (and export) shape of a dead letter (design §4.3).
type DetailView struct {
	ItemView
	Stack   string          `json:"stack"`
	Key     EncodedValue    `json:"key"`
	Value   EncodedValue    `json:"value"`
	Headers []EncodedHeader `json:"headers"`
	Masked  bool            `json:"masked"`
}

// Presenter renders records as views; Steps labels failing steps (zero value keeps step ids), Delivery is
// the route's active delivery mode (the UI warns that retries of exactly_once routes are at-least-once).
type Presenter struct {
	Steps    StepLabels
	Delivery config.DeliveryMode
}

// Item renders a list row.
func (p Presenter) Item(route RouteInfo, r record.Record, res *Resolution) ItemView {
	v := ItemView{
		ID: RefCodec{}.Encode(r.Ref), RouteID: route.ID, RouteName: route.Name, FailedAt: r.Failure.FailedAt,
		ErrorType: r.Failure.ErrorType, Message: r.Failure.Message, Step: r.Failure.Step, Branch: r.Failure.Branch,
		StepLabel: p.Steps.Of(r.Failure.VersionNo, r.Failure.Branch, r.Failure.Step),
		VersionNo: r.Failure.VersionNo, Source: SourceView{Topic: r.Source.Topic, Partition: r.Source.Partition, Offset: r.Source.Offset},
		Delivery: p.Delivery,
	}
	if res != nil {
		v.Resolution = &ResolutionView{State: res.State, By: res.By, At: res.At, Destination: res.Destination, JobID: res.JobID}
	}
	return v
}

// Detail renders the full record.
func (p Presenter) Detail(route RouteInfo, r record.Record, res *Resolution) DetailView {
	d := DetailView{ItemView: p.Item(route, r, res), Stack: r.Failure.Stack, Key: ValueEncoder{}.Encode(r.Key), Value: ValueEncoder{}.Encode(r.Value), Headers: []EncodedHeader{}}
	for _, h := range r.Headers {
		enc := ValueEncoder{}.Encode(h.Value)
		text, _ := enc.Data.(string)
		if enc.Encoding == EncodingJSON {
			enc.Encoding, text = EncodingUTF8, string(h.Value)
		}
		d.Headers = append(d.Headers, EncodedHeader{Key: h.Key, Value: text, Encoding: enc.Encoding})
	}
	return d
}

// ValueEncoder decodes bytes as JSON, else UTF-8 text, else base64 (FR-9).
type ValueEncoder struct{}

// Encode renders b; nil stays a JSON null.
func (ValueEncoder) Encode(b []byte) EncodedValue {
	switch {
	case b == nil:
		return EncodedValue{Encoding: EncodingJSON, Data: nil}
	case json.Valid(b):
		return EncodedValue{Encoding: EncodingJSON, Data: json.RawMessage(b)}
	case utf8.Valid(b):
		return EncodedValue{Encoding: EncodingUTF8, Data: string(b)}
	}
	return EncodedValue{Encoding: EncodingBase64, Data: base64.StdEncoding.EncodeToString(b)}
}

// Viewer is who looks at a dead letter.
type Viewer struct {
	UserID string
}

// ViewPolicy decides how a viewer sees a record: raw, or type-preserving masked (FR-11).
type ViewPolicy interface {
	Present(ctx context.Context, v Viewer, conn string, d DetailView) (DetailView, error)
	PresentRecord(ctx context.Context, v Viewer, conn string, rec record.Record) (record.Record, bool, error)
}

// PassThroughViewPolicy shows raw values (tests, deployments without access control).
type PassThroughViewPolicy struct{}

// Present implements ViewPolicy.
func (PassThroughViewPolicy) Present(_ context.Context, _ Viewer, _ string, d DetailView) (DetailView, error) {
	return d, nil
}

// PresentRecord implements ViewPolicy.
func (PassThroughViewPolicy) PresentRecord(_ context.Context, _ Viewer, _ string, rec record.Record) (record.Record, bool, error) {
	return rec, false, nil
}
