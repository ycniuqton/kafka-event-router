// Package secrets implements envelope encryption of connection credentials with
// KMS/Vault adapters. Postgres only ever stores ciphertext plus a secret_ref.
package secrets
