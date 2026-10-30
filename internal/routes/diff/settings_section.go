package diff

import (
	"github.com/eventrouter/router/pkg/config"
)

// SettingsMember names top-level document members of the settings section.
type SettingsMember string

const (
	MemberErrorPolicy SettingsMember = "errorPolicy"
	MemberLimits      SettingsMember = "limits"
)

// SettingsSection lists name, error policy, delivery mode and limits.
type SettingsSection struct{}

func (SettingsSection) Section() SectionKind { return SectionSettings }

func (s SettingsSection) Entries(cfg *config.RouteConfig) []Entry {
	t := Text{}
	es := Entries{}
	es.Add(string(LabelName), config.PointerRoot.Token(config.TokenName), t.Labeled(string(LabelName), cfg.Name))
	if p := cfg.ErrorPolicy; p != nil {
		at := config.PointerRoot.Key(string(MemberErrorPolicy))
		es.Add(string(LabelOnError), at, t.Labeled(string(LabelOnError), string(p.OnError)))
		es.Add(string(LabelDLQTopic), at, t.Labeled(string(LabelDLQTopic), t.Str(p.DLQTopic)))
		es.Add(string(LabelDLQConnection), at, t.Labeled(string(LabelDLQConnection), t.Str(p.DLQConnectionID)))
		es.Add(string(LabelMaxRetries), at, t.Labeled(string(LabelMaxRetries), t.Int(p.MaxRetries)))
	}
	if cfg.Delivery != nil {
		es.Add(string(LabelDelivery), config.PointerRoot.Token(config.TokenDelivery), t.Labeled(string(LabelDelivery), string(cfg.Delivery.Mode)))
	}
	s.limits(&es, cfg.Limits)
	return es
}

func (SettingsSection) limits(es *Entries, l *config.Limits) {
	if l == nil {
		return
	}
	t := Text{}
	at := config.PointerRoot.Key(string(MemberLimits))
	values := map[LimitName]*int{
		LimitJSTimeout: l.JSTimeoutMs, LimitJSMemory: l.JSMemoryMb, LimitMaxInFlight: l.MaxInFlight,
		LimitMaxOutputs: l.MaxOutputsPerEvent, LimitMaxOutputBytes: l.MaxOutputBytes,
		LimitMinInFlight: l.MinInFlight, LimitJSMinInstances: l.JSMinInstances, LimitJSMaxInstances: l.JSMaxInstances,
	}
	for _, name := range LimitNames {
		es.Add(string(KeyLimit)+string(KeySep)+string(name), at.Key(string(name)), t.Labeled(string(LabelLimit)+" "+string(name), t.Int(values[name])))
	}
	if l.InFlightMode != nil {
		name := string(LimitInFlightMode)
		es.Add(string(KeyLimit)+string(KeySep)+name, at.Key(name), t.Labeled(string(LabelLimit)+" "+name, string(*l.InFlightMode)))
	}
}

// LimitName names a member of the limits object.
type LimitName string

const (
	LimitJSTimeout      LimitName = "jsTimeoutMs"
	LimitJSMemory       LimitName = "jsMemoryMb"
	LimitMaxInFlight    LimitName = "maxInFlight"
	LimitMaxOutputs     LimitName = "maxOutputsPerEvent"
	LimitMaxOutputBytes LimitName = "maxOutputBytes"
	LimitMinInFlight    LimitName = "minInFlight"
	LimitInFlightMode   LimitName = "inFlightMode"
	LimitJSMinInstances LimitName = "jsMinInstances"
	LimitJSMaxInstances LimitName = "jsMaxInstances"
)

// LimitNames lists the numeric limit members in display order (inFlightMode is listed after them).
var LimitNames = []LimitName{LimitJSTimeout, LimitJSMemory, LimitMaxInFlight, LimitMinInFlight, LimitJSMinInstances, LimitJSMaxInstances,
	LimitMaxOutputs, LimitMaxOutputBytes}
