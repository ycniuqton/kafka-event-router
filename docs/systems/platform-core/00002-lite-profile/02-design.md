# platform-core — Design (00002-lite-profile)

Incremental version on top of `00001-hardening`. **Unchanged:** envelope encryption (`secrets.EnvelopeManager`,
per-secret data keys), provider registry, rotator, Vault / KMS providers, environment guard rules, TLS listener
code, staging compose and Helm chart. Password login design: `docs/features/access-control/00001-password-auth`.

## 1. `static_key` provider

```
internal/secrets/keyring      KeyRing: current key + previous keys by fingerprint, AES-256-GCM wrap
                              (nonce || ciphertext), Fingerprint(prefix, key) = prefix + sha256[:16 hex]
internal/secrets/local        Provider = KeyRing("local:") over a hex key file (unchanged behaviour)
internal/secrets/statickey    Provider = KeyRing("static:") over base64 env / file keys; Register(reg)
internal/platform/config      SecretsStaticKey, StaticKeyConfig (Key, KeyFile, PreviousKeys, PreviousKeyFiles),
                              guard rule (service api): provider static_key → exactly one of key / key file
cmd/api/secrets_cmd.go        SecretsAssembly registers statickey next to local, vault, kms
```

```
api start (ROUTER_SECRETS_PROVIDER=static_key)
└─ config.Loader.Load(api) → guard: zero or both key sources → start fails
└─ SecretsAssembly.Build → registry → statickey factory
   ├─ base64 invalid / not 32 bytes / unreadable file → start fails (no key bytes in the error)
   ├─ previous key invalid → start fails
   └─ KeyRing{current: static:<fp>, previous: {fp → key}} → EnvelopeManager
---
EnvelopeManager.Get(ref)
└─ provider.UnwrapKey(wrapped, keyID)
   ├─ keyID = current → open with current key
   ├─ keyID ∈ previous → open with that key (decrypt-only)
   └─ unknown keyID → error "key not available"; wrong key bytes → secrets.ErrDecrypt
---
`api secrets rotate` (CLI, or the leader rotation job)
└─ Rotator pages envelopes with key_id ≠ current → unwrap (previous) → wrap (current) → update key_id → audit
   → afterwards remove the old key from ROUTER_SECRETS_PREVIOUS_MASTER_KEYS
```

Switching an existing install from another provider kind to `static_key` is **not** a migration path
(the rotator re-wraps only envelopes the configured provider can open); the lite profile starts fresh.

## 2. Lite compose topology

```
host :WEB_PORT (8088) ──▶ web (nginx) ──https (verify internal CA, name "api")──▶ api :8080 public
                                                                                  api :8443 internal (mTLS)
worker ──https + client cert CN=worker──▶ api :8443        preview ──client cert CN=preview──▶ api :8443
api / worker / preview ──▶ postgres :5432, redis :6379     (loopback host ports only, for ssh / backups)
[profile metrics] prometheus ──scrape──▶ api (https, CA), worker :8081, preview :8082
```

```
docker compose up -d
└─ postgres, redis healthy
└─ certs-init [one-shot, network none]
   ├─ CA missing → new CA (key only in volume certsca, never mounted elsewhere)
   ├─ leaf missing / < 30 days left / not signed by current CA → issue: api (SAN api, localhost, 127.0.0.1),
   │  worker, preview (client auth) → keys 0400 uid 65532
   └─ otherwise → keep everything
└─ migrate [one-shot] `migrate up` (full api config + guards)
└─ api: TLS cert/key + ROUTER_TLS_CLIENT_CA_FILE + ROUTER_WORKLOAD_AUTH=mtls + MTLS_ALLOWED=worker,preview
   └─ healthcheck `/app health https://127.0.0.1:8080/readyz` (verifies with the internal CA)
   └─ password seed on first start (bootstrap admin from .env)
└─ worker, preview (wait for api healthy) → control plane over https://api:8443 with client certs
└─ web (waits for certs-init, api started)
```

### Decisions
- **mTLS via `certs-init`, not a weaker guard:** production rejects `static_token` and an internal listener
  without client CAs. A one-shot openssl container (like staging's `vault-init`) gives compose the same
  workload identity model as Kubernetes deployments with mTLS, and the same setup works in trial mode.
- **The public listener becomes HTTPS too:** the api shares one TLS config for both listeners
  (`internal/platform/app/app.go`, `internal/platform/httpserver/tls.go`); client certificates are demanded
  only on :8443. nginx therefore proxies to `https://api:8080` and verifies against the internal CA.
- **`api health` subcommand** instead of busybox `wget` for the HTTPS healthcheck (TLS support of the
  distroless debug busybox is not guaranteed).
- **Cookie security:** production keeps `ROUTER_COOKIE_SECURE=true`, so the browser must reach the stack over
  HTTPS: an external terminator (recommended) or `docker-compose.tls.yml` (nginx terminates TLS with mounted
  certificates). Trial mode = `ROUTER_ENV=staging` + insecure cookies (guards allow it outside production).
- **Metrics off:** there is no `none` metrics backend (only `prometheus`, `mock`; `mock` would show fake
  numbers). The backend stays `prometheus` at an absent host and fails fast; `ROUTER_ROUTE_METRICS_READER=null`,
  `ROUTER_ALERTS_ENABLED=false` and attention detectors without metric-based kinds avoid log noise.
  `init.sh --metrics` switches all of them together with `COMPOSE_PROFILES=metrics`.
- **No platform Kafka:** `ROUTER_KAFKA_BOOTSTRAP` is unused outside `cmd/stresssweep`; route clusters come from
  user connections.

## 3. Operations
- **DR backup:** `pg_dump` of the router database + the `.env` (above all `ROUTER_SECRETS_MASTER_KEY` and any
  previous keys). Without the master key every saved connection credential is unrecoverable. The `certs` /
  `certsca` volumes can be regenerated (certs-init issues new ones when the CA is gone).
- **Key rotation:** current key → `ROUTER_SECRETS_PREVIOUS_MASTER_KEYS`, new key in `ROUTER_SECRETS_MASTER_KEY`,
  restart api, `docker compose exec api /app secrets rotate`, check `/app secrets status`, drop the old key.

## 4. Known limits
- Single `certs` volume: every service runs as uid 65532 and can read every leaf key on the volume
  (worker could read the api / preview keys). Acceptable on one host; split volumes if this matters.
- Behind a TLS terminator, nginx sees the proxy address: the per-IP login limit counts per proxy unless
  `set_real_ip_from` / `real_ip_header` are configured (README).
- `certs-init` uses `dockerfile_inline` (Compose ≥ 2.17) and installs openssl at build time (network needed).
- Not proven without a live run: nginx HTTPS upstream, the TLS override, Prometheus HTTPS scrape.

## 5. Tests
Unit: `internal/secrets/statickey` (roundtrip, wrong key, missing/short/invalid/both keys, key file,
previous keys + rotation via the rotator), `internal/secrets/local` (key id pinned), `internal/platform/config`
(static_key in production for api, worker without key, local still local-only), `cmd/api/health_cmd_test.go`.
Static: `deploy/lite/validate.py` (YAML, references, interpolation vs `.env.example` / `init.sh`, every
`ROUTER_*` key read by Go code, volumes / networks / profiles, healthcheck binaries, loopback ports, key shape,
`sh -n`); `init.sh` generation + idempotency; real Go config loaders + guards fed with each service's env.
