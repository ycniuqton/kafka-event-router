// Package testkit is the shared integration-test harness: it provides Postgres, Redis, Kafka and
// Vault endpoints from a config-selected Provider (external services or throwaway containers) and
// isolated Postgres sandboxes (one schema per test, migrated with the embedded migrations).
package testkit
