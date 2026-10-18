package connections

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/schemaregistry"
)

// RuleMessage holds the user-facing texts of connection validation.
type RuleMessage string

const (
	MsgNameRequired    RuleMessage = "Enter a name."
	MsgNameInvalid     RuleMessage = "Use up to 100 letters, digits, spaces, dots, dashes or underscores."
	MsgDescriptionLong RuleMessage = "Keep the description under 1000 characters."
	MsgEnvInvalid      RuleMessage = "Pick dev, staging or prod."
	MsgBrokersRequired RuleMessage = "Add at least one bootstrap server."
	MsgBrokersTooMany  RuleMessage = "Add at most 50 bootstrap servers."
	MsgBrokerInvalid   RuleMessage = "host:port expected, for example kafka-1.internal:9092."
	MsgBrokerDuplicate RuleMessage = "This server is listed twice."
	MsgKindInvalid     RuleMessage = "Only kafka connections can be created here."
)

// ConnectionRule is the base of every draft validation rule.
type ConnectionRule interface {
	Check(ctx context.Context, d Draft) []kafka.FieldError
}

// NamePattern is the allowed shape of connection names.
var NamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`)

// NameRule checks name and description.
type NameRule struct{}

// Check implements ConnectionRule.
func (NameRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	var errs []kafka.FieldError
	name := strings.TrimSpace(d.Name)
	if name == "" {
		errs = append(errs, kafka.FieldError{Path: string(PathName), Code: kafka.CodeRequired, Message: string(MsgNameRequired)})
	} else if !NamePattern.MatchString(name) {
		errs = append(errs, kafka.FieldError{Path: string(PathName), Code: kafka.CodeInvalid, Message: string(MsgNameInvalid)})
	}
	if len(d.Description) > int(MaxDescriptionLength) {
		errs = append(errs, kafka.FieldError{Path: string(PathDescription), Code: kafka.CodeInvalid, Message: string(MsgDescriptionLong)})
	}
	return errs
}

// EnvRule checks the env tag and cluster kind.
type EnvRule struct{}

// Check implements ConnectionRule.
func (EnvRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	var errs []kafka.FieldError
	if !d.EnvTag.Valid() {
		errs = append(errs, kafka.FieldError{Path: string(PathEnvTag), Code: kafka.CodeInvalid, Message: string(MsgEnvInvalid)})
	}
	if d.KindOrDefault() != kafka.ClusterKafka {
		errs = append(errs, kafka.FieldError{Path: string(PathKind), Code: kafka.CodeInvalid, Message: string(MsgKindInvalid)})
	}
	return errs
}

// BootstrapRule checks the seed list: 1..50 unique host:port entries.
type BootstrapRule struct{}

// Check implements ConnectionRule.
func (r BootstrapRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	path := string(kafka.PathBootstrapServers)
	if len(d.BootstrapServers) == 0 {
		return []kafka.FieldError{{Path: path, Code: kafka.CodeRequired, Message: string(MsgBrokersRequired)}}
	}
	if len(d.BootstrapServers) > int(MaxBootstrapServers) {
		return []kafka.FieldError{{Path: path, Code: kafka.CodeInvalid, Message: string(MsgBrokersTooMany)}}
	}
	var errs []kafka.FieldError
	seen := map[string]bool{}
	for i, s := range d.BootstrapServers {
		at := fmt.Sprintf("%s/%d", path, i)
		if !(HostPort{}).Valid(s) {
			errs = append(errs, kafka.FieldError{Path: at, Code: kafka.CodeInvalid, Message: string(MsgBrokerInvalid)})
		} else if seen[s] {
			errs = append(errs, kafka.FieldError{Path: at, Code: kafka.CodeInvalid, Message: string(MsgBrokerDuplicate)})
		}
		seen[s] = true
	}
	return errs
}

// HostPort validates "host:port" seeds.
type HostPort struct{}

// Valid reports whether s is a host (or IP) with a port in 1..65535.
func (HostPort) Valid(s string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(s))
	if err != nil || host == "" || strings.ContainsAny(host, " /") {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= int(MaxPort)
}

// SecurityRule delegates to the security-mode Manager.
type SecurityRule struct {
	Registry *kafka.SecurityRegistry
}

// Check implements ConnectionRule.
func (r SecurityRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	return r.Registry.Validate(d.Spec())
}

// PropertiesRule delegates to the advanced-property allow-list.
type PropertiesRule struct {
	Catalog *kafka.PropertyCatalog
}

// Check implements ConnectionRule.
func (r PropertiesRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	return r.Catalog.Validate(d.Properties)
}

// SchemaRegistryRule delegates to the registry auth Manager (URL + per-mode fields); absent registry is valid.
type SchemaRegistryRule struct {
	Registry *schemaregistry.AuthRegistry
}

// Check implements ConnectionRule.
func (r SchemaRegistryRule) Check(_ context.Context, d Draft) []kafka.FieldError {
	return r.Registry.Validate(d.Spec())
}

// Validator is the Manager running the configured rules and aggregating a 422 problem.
type Validator struct {
	rules []ConnectionRule
}

// NewValidator builds a validator over rules (run in order).
func NewValidator(rules ...ConnectionRule) *Validator {
	return &Validator{rules: rules}
}

// DefaultValidator wires the full rule set (schema registry rule with the default auth modes).
func DefaultValidator(security *kafka.SecurityRegistry, props *kafka.PropertyCatalog) *Validator {
	return NewValidator(NameRule{}, EnvRule{}, BootstrapRule{}, SecurityRule{Registry: security}, PropertiesRule{Catalog: props},
		SchemaRegistryRule{Registry: schemaregistry.DefaultAuthRegistry()})
}

// Validate returns a *api.ValidationError (first error = problem path) or nil.
func (v *Validator) Validate(ctx context.Context, d Draft) error {
	verr := api.NewValidationError()
	for _, rule := range v.rules {
		for _, fe := range rule.Check(ctx, d) {
			verr.Add(fe.Path, string(fe.Code), fe.Message)
		}
	}
	return verr.OrNil()
}
