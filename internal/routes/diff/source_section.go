package diff

import (
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// SourcePointer addresses the source object.
var SourcePointer = config.PointerRoot.Token(config.TokenSource)

// SourceSection lists connection, topics, start position, formats and consumer group.
type SourceSection struct{}

func (SourceSection) Section() SectionKind { return SectionSource }

func (SourceSection) Entries(cfg *config.RouteConfig) []Entry {
	t, s := Text{}, cfg.Source
	es := Entries{}
	es.Add(string(LabelConnection), SourcePointer.Token(config.TokenConnection), t.Labeled(string(LabelConnection), s.ConnectionID))
	for i, topic := range s.Topics {
		es.Add(string(KeyTopic)+string(KeySep)+topic, SourcePointer.Key(string(KeyTopics)).Index(i), t.Labeled(string(LabelTopic), topic))
	}
	es.Add(string(LabelTopicPattern), SourcePointer, t.Labeled(string(LabelTopicPattern), t.Str(s.TopicPattern)))
	if s.StartFrom != nil {
		es.Add(string(LabelStartFrom), SourcePointer, t.Labeled(string(LabelStartFrom), string(s.StartFrom.Mode)+" "+t.JSON(s.StartFrom.At)+" "+t.JSON(s.StartFrom.Offsets)))
	}
	es.Add(string(LabelFormat), SourcePointer.Token(config.TokenFormat), t.Labeled(string(LabelFormat), FormatText{}.Of(s.Format)))
	if s.KeyFormat != nil {
		es.Add(string(LabelKeyFormat), SourcePointer.Token(config.TokenKeyFormat), t.Labeled(string(LabelKeyFormat), FormatText{}.Of(*s.KeyFormat)))
	}
	es.Add(string(LabelConsumerGroup), SourcePointer, t.Labeled(string(LabelConsumerGroup), t.Str(s.ConsumerGroup)))
	return es
}

// FormatText renders a source format with its schema subject and message type.
type FormatText struct{}

// Of returns "avro orders-value acme.Order" style text.
func (FormatText) Of(f config.Format) string {
	t := Text{}
	return strings.Join(strings.Fields(string(f.Type)+" "+t.Str(f.Subject)+" "+t.Str(f.MessageType)), " ")
}
