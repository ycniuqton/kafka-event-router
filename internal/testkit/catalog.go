package testkit

import "strings"

// HostPortToken is replaced in ContainerSpec env and URL templates by the published host address.
const HostPortToken = "{hostport}"

// ContainerSpec describes how to run one service as a throwaway container.
type ContainerSpec struct {
	Image string
	Port  string
	Env   []string
	URL   string
	Token string
}

// Render substitutes the published host address into the spec's env and URL.
func (s ContainerSpec) Render(hostPort string) (env []string, url string) {
	for _, e := range s.Env {
		env = append(env, strings.ReplaceAll(e, HostPortToken, hostPort))
	}
	return env, strings.ReplaceAll(s.URL, HostPortToken, hostPort)
}

// ContainerCatalog is the image catalogue of the container provider (same images as compose).
type ContainerCatalog map[Service]ContainerSpec

// DefaultContainerCatalog mirrors deploy/docker-compose.yml so tests and the local stack agree.
func DefaultContainerCatalog() ContainerCatalog {
	return ContainerCatalog{
		ServicePostgres: {
			Image: "postgres:16-alpine", Port: "5432",
			Env: []string{"POSTGRES_PASSWORD=testkit", "POSTGRES_USER=testkit", "POSTGRES_DB=testkit"},
			URL: "postgres://testkit:testkit@" + HostPortToken + "/testkit?sslmode=disable",
		},
		ServiceRedis: {Image: "redis:7-alpine", Port: "6379", URL: "redis://" + HostPortToken + "/0"},
		ServiceKafka: {
			Image: "apache/kafka:3.9.1", Port: "9092",
			Env: []string{
				"KAFKA_NODE_ID=1", "KAFKA_PROCESS_ROLES=broker,controller",
				"KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093",
				"KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://" + HostPortToken,
				"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT",
				"KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER", "KAFKA_CONTROLLER_QUORUM_VOTERS=1@localhost:9093",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1", "KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1",
				"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1", "KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0",
			},
			URL: HostPortToken,
		},
		ServiceVault: {
			Image: "hashicorp/vault:1.17", Port: "8200",
			Env: []string{"VAULT_DEV_ROOT_TOKEN_ID=testkit", "VAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200"},
			URL: "http://" + HostPortToken, Token: "testkit",
		},
		ServiceSchemaRegistry: {
			Image: "apicurio/apicurio-registry-mem:2.6.2.Final", Port: "8080",
			URL: "http://" + HostPortToken + "/apis/ccompat/v7",
		},
	}
}
