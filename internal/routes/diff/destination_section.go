package diff

import (
	"github.com/eventrouter/router/pkg/config"
)

// DestinationSection lists each branch's connection, topic, key, partitioning, headers and format.
type DestinationSection struct{}

func (DestinationSection) Section() SectionKind { return SectionDestination }

func (DestinationSection) Entries(cfg *config.RouteConfig) []Entry {
	es := Entries{}
	for i, b := range cfg.Branches {
		DestinationLines{Prefix: BranchLabel{}.For(cfg, b), Key: b.ID, At: config.BranchPointer(i).Token(config.TokenDestination)}.Add(&es, b.Destination)
	}
	return es
}

// DestinationLines renders one destination.
type DestinationLines struct {
	Prefix string
	Key    string
	At     config.Pointer
}

// Add appends the destination's entries.
func (d DestinationLines) Add(es *Entries, dst config.Destination) {
	t := Text{}
	d.add(es, LabelConnection, d.At.Token(config.TokenConnection), dst.ConnectionID)
	d.add(es, LabelTopic, d.At.Key(string(KeyTopic)), dst.Topic)
	if dst.Key != nil {
		d.add(es, LabelKey, d.At.Token(config.TokenKey), string(dst.Key.Mode)+" "+t.Str(dst.Key.Field))
	}
	if dst.Partitioning != nil {
		d.add(es, LabelPartitioning, d.At, string(*dst.Partitioning))
	}
	if dst.Headers != nil {
		d.add(es, LabelHeaders, d.At, string(dst.Headers.Mode)+" "+t.JSON(dst.Headers.Keep)+" "+t.JSON(dst.Headers.Add))
	}
	if dst.Format != nil {
		d.add(es, LabelFormat, d.At.Token(config.TokenFormat), string(dst.Format.Type)+" "+t.Str(dst.Format.Subject))
	}
	if dst.CreateTopic != nil {
		d.add(es, LabelCreateTopic, d.At, t.Int(&dst.CreateTopic.Partitions))
	}
}

func (d DestinationLines) add(es *Entries, label Label, at config.Pointer, value string) {
	es.AddPrefixed(d.Key+string(KeySep)+string(label), at, d.Prefix, Text{}.Labeled(string(label), Text{}.Truncate(value)))
}
