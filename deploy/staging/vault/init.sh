#!/bin/sh
# Vault init/unseal sidecar: initialises once (1 key share) into /state/vault-init.json, unseals on every
# start and whenever Vault reports sealed, and idempotently provisions transit + the app token.
set -u

STATE=/state/vault-init.json
READY=/tmp/vault-ready
POLICY=event-router-transit
LOOP_SECONDS=15
TOKEN_TTL=87600h
PROVISIONED=0

# Reads one JSON string field from the init file (no jq in the vault image).
json_field() {
  tr -d '\n ' < "$STATE" | sed -n "s/.*\"$1\":\[\{0,1\}\"\([^\"]*\)\".*/\1/p"
}

# Reports a boolean field of `vault status`.
status_field() {
  vault status -format=json 2>/dev/null | tr -d '\n ' | sed -n "s/.*\"$1\":\([a-z]*\).*/\1/p"
}

# Runs `vault operator init` once; refuses to re-init when keys already exist.
init_vault() {
  if [ "$(status_field initialized)" = "true" ]; then
    return 0
  fi
  if [ -s "$STATE" ]; then
    echo "vault-init: vault uninitialised but $STATE is not empty; refusing to overwrite" >&2
    return 1
  fi
  vault operator init -key-shares=1 -key-threshold=1 -format=json > "$STATE" || return 1
  echo "vault-init: initialised, keys written to vault-init.json"
}

# Unseals with the stored key when sealed.
unseal_vault() {
  if [ "$(status_field sealed)" = "true" ]; then
    vault operator unseal "$(json_field unseal_keys_b64)" > /dev/null || return 1
    echo "vault-init: unsealed"
  fi
}

# Enables transit, creates the key, policy and the fixed-id app token if any is missing.
provision() {
  export VAULT_TOKEN="$(json_field root_token)"
  vault secrets list -format=json | grep -q "\"${TRANSIT_MOUNT}/\"" || vault secrets enable -path="$TRANSIT_MOUNT" transit || return 1
  vault read "${TRANSIT_MOUNT}/keys/${TRANSIT_KEY}" > /dev/null 2>&1 || vault write -f "${TRANSIT_MOUNT}/keys/${TRANSIT_KEY}" > /dev/null || return 1
  printf 'path "%s/encrypt/%s" { capabilities = ["update"] }\npath "%s/decrypt/%s" { capabilities = ["update"] }\npath "%s/keys/%s" { capabilities = ["read"] }\n' \
    "$TRANSIT_MOUNT" "$TRANSIT_KEY" "$TRANSIT_MOUNT" "$TRANSIT_KEY" "$TRANSIT_MOUNT" "$TRANSIT_KEY" | vault policy write "$POLICY" - > /dev/null || return 1
  vault write sys/auth/token/tune max_lease_ttl="$TOKEN_TTL" default_lease_ttl="$TOKEN_TTL" > /dev/null || return 1
  vault token lookup "$ROUTER_VAULT_TOKEN" > /dev/null 2>&1 || vault token create -id="$ROUTER_VAULT_TOKEN" -policy="$POLICY" -orphan -ttl="$TOKEN_TTL" > /dev/null || return 1
  unset VAULT_TOKEN
}

# Provisions once per container start (cheap re-checks are not needed while Vault stays up).
provision_once() {
  [ "$PROVISIONED" = "1" ] && return 0
  provision && PROVISIONED=1
}

# One reconcile pass; the ready marker exists only while Vault is unsealed and provisioned.
reconcile() {
  init_vault && unseal_vault && provision_once && touch "$READY" && return 0
  rm -f "$READY"
  return 1
}

while true; do
  reconcile || echo "vault-init: reconcile failed, retrying" >&2
  sleep "$LOOP_SECONDS"
done
