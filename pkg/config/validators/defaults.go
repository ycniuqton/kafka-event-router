package validators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// Deps are the ports the doc-only validators need (the compile dry-run's sandbox and library).
type Deps struct {
	Compiler engine.Compiler
	Options  engine.CompileOptions
}

// NewRegistry registers schema, delivery_mode and compile validators.
func NewRegistry(d Deps) (*config.ValidatorRegistry, error) {
	schema, err := NewSchemaValidator()
	if err != nil {
		return nil, err
	}
	return config.NewValidatorRegistry(schema, DeliveryModeValidator{}, CompileValidator{Compiler: d.Compiler, Options: d.Options}), nil
}

// NewManager builds a ValidatorManager from a profile over the doc-only registry.
func NewManager(profile config.ValidationProfile, d Deps) (*config.ValidatorManager, error) {
	reg, err := NewRegistry(d)
	if err != nil {
		return nil, err
	}
	return config.NewValidatorManager(profile, reg)
}

// NewDefaultManager builds the manager with config.DefaultValidationProfile.
func NewDefaultManager(d Deps) (*config.ValidatorManager, error) {
	return NewManager(config.DefaultValidationProfile(), d)
}
