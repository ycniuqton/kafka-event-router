package routes

import (
	"bytes"
	"encoding/json"

	"github.com/eventrouter/router/pkg/config"
)

// DocumentKey names the route document members the draft editor touches.
type DocumentKey string

const (
	KeyName          DocumentKey = "name"
	KeySource        DocumentKey = "source"
	KeyConsumerGroup DocumentKey = "consumerGroup"
)

// JSONMarker names structural bytes of JSON text.
type JSONMarker byte

const jsonObjectStart JSONMarker = '{'

// DraftEditor applies server-side defaults to drafts without touching anything else.
type DraftEditor struct{}

// IsObject reports whether raw is a parseable JSON object.
func (DraftEditor) IsObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || JSONMarker(trimmed[0]) != jsonObjectStart {
		return false
	}
	_, err := config.Parse(trimmed)
	return err == nil
}

// WithDefaults sets name and source.consumerGroup when the draft omits them.
func (e DraftEditor) WithDefaults(raw json.RawMessage, routeName string) (json.RawMessage, error) {
	return e.edit(raw, func(doc map[string]any, src map[string]any) {
		if _, ok := doc[string(KeyName)]; !ok {
			doc[string(KeyName)] = routeName
		}
		if _, ok := src[string(KeyConsumerGroup)]; !ok && src != nil {
			src[string(KeyConsumerGroup)] = ConsumerGroupNamer{}.For(routeName)
		}
	})
}

// KeepConsumerGroup gives a saved draft that omits source.consumerGroup the group of the previous draft
// (else the route's default), so saving a draft never moves the route to a new group that re-reads the topic.
func (e DraftEditor) KeepConsumerGroup(raw, previous json.RawMessage, routeName string) (json.RawMessage, error) {
	group := ConsumerGroupNamer{}.For(routeName)
	if prev := (DraftEditor{}).consumerGroup(previous); prev != "" {
		group = prev
	}
	return e.edit(raw, func(_ map[string]any, src map[string]any) {
		if _, ok := src[string(KeyConsumerGroup)]; !ok && src != nil {
			src[string(KeyConsumerGroup)] = group
		}
	})
}

// consumerGroup reads source.consumerGroup of raw ("" when absent or unreadable).
func (DraftEditor) consumerGroup(raw json.RawMessage) string {
	var doc struct {
		Source struct {
			ConsumerGroup string `json:"consumerGroup"`
		} `json:"source"`
	}
	if raw == nil || json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return doc.Source.ConsumerGroup
}

// Retarget renames a copied document and gives it its own default consumer group.
func (e DraftEditor) Retarget(raw json.RawMessage, routeName string) (json.RawMessage, error) {
	return e.edit(raw, func(doc map[string]any, src map[string]any) {
		doc[string(KeyName)] = routeName
		if src != nil {
			src[string(KeyConsumerGroup)] = ConsumerGroupNamer{}.For(routeName)
		}
	})
}

// edit decodes raw keeping number precision, applies fn and re-encodes.
func (DraftEditor) edit(raw json.RawMessage, fn func(doc, src map[string]any)) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	var doc map[string]any
	if err := (config.JSONDecoder{}).Into(raw, &doc); err != nil {
		return nil, err
	}
	src, _ := doc[string(KeySource)].(map[string]any)
	fn(doc, src)
	return json.Marshal(doc)
}

// ConfigOf decodes the typed config of a raw document; nil when it cannot be decoded.
func ConfigOf(raw json.RawMessage) *config.RouteConfig {
	if raw == nil {
		return nil
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return nil
	}
	cfg, err := doc.Config()
	if err != nil {
		return nil
	}
	return cfg
}
