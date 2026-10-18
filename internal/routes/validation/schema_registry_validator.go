package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// Issue codes of the format rules.
const (
	IssueSchemaRegistryMissing config.IssueCode = "schema_registry_missing"
	IssueMessageTypeDefault    config.IssueCode = "message_type_default"
)

// registryFormats are the formats that need the connection's schema registry.
var registryFormats = map[string]bool{string(config.FormatTypeAvro): true, string(config.FormatTypeProtobuf): true}

// FormatUse is one format setting that may need a schema registry.
type FormatUse struct {
	ConnectionID string
	Type         string
	MessageType  *string
	Path         config.Pointer
	Destination  bool
}

// FormatUses collects source value/key formats and branch destination formats.
type FormatUses struct{}

// Of returns the format settings of cfg that name a registry format.
func (FormatUses) Of(cfg *config.RouteConfig) []FormatUse {
	src := config.PointerRoot.Token(config.TokenSource)
	out := []FormatUse{{ConnectionID: cfg.Source.ConnectionID, Type: string(cfg.Source.Format.Type), MessageType: cfg.Source.Format.MessageType, Path: src.Token(config.TokenFormat)}}
	if k := cfg.Source.KeyFormat; k != nil {
		out = append(out, FormatUse{ConnectionID: cfg.Source.ConnectionID, Type: string(k.Type), MessageType: k.MessageType, Path: src.Token(config.TokenKeyFormat)})
	}
	for i, b := range cfg.Branches {
		if f := b.Destination.Format; f != nil {
			out = append(out, FormatUse{ConnectionID: b.Destination.ConnectionID, Type: string(f.Type), MessageType: f.MessageType,
				Path: config.BranchPointer(i).Token(config.TokenDestination).Token(config.TokenFormat), Destination: true})
		}
	}
	kept := []FormatUse{}
	for _, u := range out {
		if registryFormats[u.Type] && u.ConnectionID != "" {
			kept = append(kept, u)
		}
	}
	return kept
}

// SchemaRegistryValidator checks that Avro / Protobuf formats are used on connections with a schema registry.
type SchemaRegistryValidator struct {
	Lookup ConnectionLookup
}

func (SchemaRegistryValidator) Name() ValidatorName     { return ValidatorSchemaRegistry }
func (SchemaRegistryValidator) Modes() []ValidationMode { return AllModes }

// Validate: registry missing → error; Protobuf output without messageType → warning (first message is used).
func (v SchemaRegistryValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	uses := FormatUses{}.Of(in.Config)
	if len(uses) == 0 {
		return nil, nil
	}
	ids := []string{}
	for _, u := range uses {
		ids = append(ids, u.ConnectionID)
	}
	found, err := v.Lookup.Find(ctx, ids)
	if err != nil {
		return nil, err
	}
	issues := []config.Issue{}
	for _, u := range uses {
		issues = append(issues, v.check(u, found)...)
	}
	return issues, nil
}

// check validates one format use; unknown connections are the connection validator's issue.
func (SchemaRegistryValidator) check(u FormatUse, found map[string]ConnectionInfo) []config.Issue {
	conn, ok := found[u.ConnectionID]
	if !ok {
		return nil
	}
	if !conn.HasSchemaRegistry {
		return []config.Issue{config.NewError(IssueSchemaRegistryMissing, u.Path,
			"Connection %q has no schema registry, which %s needs. Add the registry to the connection or use JSON.", conn.Name, u.Type)}
	}
	if u.Destination && u.Type == string(config.FormatTypeProtobuf) && (u.MessageType == nil || *u.MessageType == "") {
		return []config.Issue{config.NewWarning(IssueMessageTypeDefault, u.Path,
			"No message type set: the first message of the subject's schema is used.")}
	}
	return nil
}
