package testkit

// Service names a backing service an integration test can require.
type Service string

const (
	ServicePostgres Service = "postgres"
	ServiceRedis    Service = "redis"
	ServiceKafka    Service = "kafka"
	ServiceVault    Service = "vault"

	ServiceSchemaRegistry Service = "schema-registry"
)

// ProviderKind names a Provider implementation; ROUTER_TEST_PROVIDER selects one.
type ProviderKind string

const (
	ProviderExternal  ProviderKind = "external"
	ProviderContainer ProviderKind = "container"
	DefaultProvider   ProviderKind = ProviderExternal
)

// EnvKey names the environment variables the harness reads.
type EnvKey string

const (
	EnvProvider    EnvKey = "ROUTER_TEST_PROVIDER"
	EnvPostgresURL EnvKey = "ROUTER_TEST_DATABASE_URL"
	EnvRedisURL    EnvKey = "ROUTER_TEST_REDIS_URL"
	EnvKafka       EnvKey = "ROUTER_TEST_KAFKA_BOOTSTRAP"
	EnvVaultAddr   EnvKey = "ROUTER_TEST_VAULT_ADDR"
	EnvVaultToken  EnvKey = "ROUTER_TEST_VAULT_TOKEN"

	EnvSchemaRegistryURL EnvKey = "ROUTER_TEST_SCHEMA_REGISTRY_URL"
)

// Endpoint is how a test reaches a provided service: a URL/address plus an optional token.
type Endpoint struct {
	Service Service
	URL     string
	Token   string
}
