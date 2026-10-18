package preview

import (
	"fmt"
	"os"
	"strconv"

	"github.com/eventrouter/router/internal/platform/config"
)

// Env keys of live preview (read by both cmd/api and cmd/preview).
const (
	EnvPreviewURL          config.EnvKey = "ROUTER_PREVIEW_URL"
	EnvSessionStore        config.EnvKey = "ROUTER_PREVIEW_SESSION_STORE"
	EnvMaxRuns             config.EnvKey = "ROUTER_PREVIEW_MAX_RUNS"
	EnvDirectiveSecret     config.EnvKey = "ROUTER_PREVIEW_DIRECTIVE_SECRET"
	EnvKafkaSampler        config.EnvKey = "ROUTER_PREVIEW_KAFKA_SAMPLER"
	EnvScripts             config.EnvKey = "ROUTER_PREVIEW_SCRIPTS"
	EnvSchemaInferenceJobs config.EnvKey = "ROUTER_PREVIEW_SCHEMA_JOBS"
	EnvSalts               config.EnvKey = "ROUTER_PREVIEW_SALTS"
	EnvRatePerMinute       config.EnvKey = "ROUTER_PREVIEW_RATE_PER_MINUTE"
	EnvRateBurst           config.EnvKey = "ROUTER_PREVIEW_RATE_BURST"
	EnvTestTopicRegistry   config.EnvKey = "ROUTER_PREVIEW_TEST_TOPICS"
	EnvSchemaRegistry      config.EnvKey = "ROUTER_PREVIEW_SCHEMA_REGISTRY"
)

// TestTopicRegistryKind selects where test topics are recorded for the janitor.
type TestTopicRegistryKind string

const (
	TestTopicsPostgres TestTopicRegistryKind = "postgres"
	TestTopicsMemory   TestTopicRegistryKind = "memory"
)

// SchemaRegistryKind selects how preview resolves the schema registry of a connection.
type SchemaRegistryKind string

const (
	SchemaRegistryControlPlane SchemaRegistryKind = "control_plane"
	SchemaRegistryNone         SchemaRegistryKind = "none"
)

// StoreKind selects the SessionStore implementation.
type StoreKind string

const (
	StoreRedis  StoreKind = "redis"
	StoreMemory StoreKind = "memory"
)

// SamplerKind selects the Kafka sampling client implementation.
type SamplerKind string

const (
	SamplerUnavailable SamplerKind = "unavailable"
	SamplerFranz       SamplerKind = "franz"
)

// ScriptsKind selects the JS runtime used for js steps and expr filters.
type ScriptsKind string

const (
	ScriptsSandbox ScriptsKind = "sandbox"
	ScriptsNone    ScriptsKind = "none"
)

// SettingDefault holds the defaults of preview settings.
type SettingDefault string

const (
	DefaultPreviewURL      SettingDefault = "http://localhost:8082"
	DefaultStore           SettingDefault = SettingDefault(StoreRedis)
	DefaultSampler         SettingDefault = SettingDefault(SamplerFranz)
	DefaultScripts         SettingDefault = SettingDefault(ScriptsSandbox)
	DefaultDirectiveSecret SettingDefault = "local-dev-only-preview-directive-secret"
	DefaultLocalSalts      SettingDefault = SettingDefault(SaltsProcess)
	DefaultRemoteSalts     SettingDefault = SettingDefault(SaltsControlPlane)
	DefaultTestTopics      SettingDefault = SettingDefault(TestTopicsPostgres)
	DefaultSchemaRegistry  SettingDefault = SettingDefault(SchemaRegistryControlPlane)
)

// SettingNumber holds numeric defaults of preview settings.
type SettingNumber int

const (
	DefaultMaxRuns    SettingNumber = 8
	DefaultSchemaJobs SettingNumber = 4
	DefaultRatePerMin SettingNumber = 240
	DefaultRateBurst  SettingNumber = 40
)

// Settings is the live-preview configuration.
type Settings struct {
	URL             string
	SessionStore    StoreKind
	MaxRuns         int
	DirectiveSecret string
	KafkaSampler    SamplerKind
	Scripts         ScriptsKind
	SchemaJobs      int
	Salts           SaltsKind
	RatePerMinute   int
	RateBurst       int
	TestTopics      TestTopicRegistryKind
	SchemaRegistry  SchemaRegistryKind
}

// SettingsLoader reads Settings from the environment; remote environments need a real secret.
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// NewSettingsLoader reads from the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load builds the settings for the given environment.
func (l SettingsLoader) Load(env config.Environment) (Settings, error) {
	maxRuns, err := l.number(EnvMaxRuns, DefaultMaxRuns)
	if err != nil {
		return Settings{}, err
	}
	jobs, err := l.number(EnvSchemaInferenceJobs, DefaultSchemaJobs)
	if err != nil {
		return Settings{}, err
	}
	rate, err := l.rate()
	if err != nil {
		return Settings{}, err
	}
	s := Settings{
		URL:             l.str(EnvPreviewURL, DefaultPreviewURL),
		SessionStore:    StoreKind(l.str(EnvSessionStore, DefaultStore)),
		MaxRuns:         maxRuns,
		DirectiveSecret: l.str(EnvDirectiveSecret, ""),
		KafkaSampler:    SamplerKind(l.str(EnvKafkaSampler, DefaultSampler)),
		Scripts:         ScriptsKind(l.str(EnvScripts, DefaultScripts)),
		SchemaJobs:      jobs,
		Salts:           SaltsKind(l.str(EnvSalts, l.saltsDefault(env))),
		RatePerMinute:   rate.PerMinute,
		RateBurst:       rate.Burst,
		TestTopics:      TestTopicRegistryKind(l.str(EnvTestTopicRegistry, DefaultTestTopics)),
		SchemaRegistry:  SchemaRegistryKind(l.str(EnvSchemaRegistry, DefaultSchemaRegistry)),
	}
	if s.DirectiveSecret == "" && env != config.EnvironmentLocal {
		return Settings{}, fmt.Errorf("preview: %s is required outside local", EnvDirectiveSecret)
	}
	if s.DirectiveSecret == "" {
		s.DirectiveSecret = string(DefaultDirectiveSecret)
	}
	return s, nil
}

// RateSetting is the per-user preview rate.
type RateSetting struct{ PerMinute, Burst int }

// rate reads the per-user preview rate.
func (l SettingsLoader) rate() (RateSetting, error) {
	perMinute, err := l.number(EnvRatePerMinute, DefaultRatePerMin)
	if err != nil {
		return RateSetting{}, err
	}
	burst, err := l.number(EnvRateBurst, DefaultRateBurst)
	return RateSetting{PerMinute: perMinute, Burst: burst}, err
}

// saltsDefault keeps local runs self-contained and reads team salts from the API elsewhere.
func (l SettingsLoader) saltsDefault(env config.Environment) SettingDefault {
	if env == config.EnvironmentLocal {
		return DefaultLocalSalts
	}
	return DefaultRemoteSalts
}

func (l SettingsLoader) str(key config.EnvKey, def SettingDefault) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return string(def)
}

func (l SettingsLoader) number(key config.EnvKey, def SettingNumber) (int, error) {
	v, ok := l.Lookup(string(key))
	if !ok || v == "" {
		return int(def), nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("preview: %s must be a positive integer, got %q", key, v)
	}
	return n, nil
}

// TestTopicStore returns only the configured test-topic registry kind (needed before infrastructure opens).
func (l SettingsLoader) TestTopicStore() TestTopicRegistryKind {
	return TestTopicRegistryKind(l.str(EnvTestTopicRegistry, DefaultTestTopics))
}

// Store returns only the configured session store kind (needed before infrastructure opens).
func (l SettingsLoader) Store() StoreKind {
	return StoreKind(l.str(EnvSessionStore, DefaultStore))
}
