# platform-core — Plan (00002-lite-profile)

Incremental version on top of `00001-hardening`. **Unchanged:** every PC-xx / PC-HD-xx item keeps its state in
its own version. Mark the step you start `[~]`, finished `[x]`; never delete items (skipped → inline note).
Password auth items live in `docs/features/access-control/00001-password-auth/03-plan.md`.

## BE: `static_key` secrets provider
- [x] PC-LT-01 Config: `SecretsStaticKey`, `ROUTER_SECRETS_MASTER_KEY(_FILE)`, previous keys; guard (api only: key required; `local` stays local-only) — `internal/platform/config`
- [x] PC-LT-02 Provider `internal/secrets/statickey` (AES-256-GCM wrap, fingerprint key id), shared code with `internal/secrets/local`
- [x] PC-LT-03 Previous keys decrypt-only → `api secrets rotate` re-wraps to the current key
- [x] PC-LT-04 Unit tests: roundtrip, wrong key, short/missing key refuses start, rotation

## Deploy: `deploy/lite`
- [x] PC-LT-05 `docker-compose.yml`: postgres 16, redis 7, migrate, api, worker, preview, web; `--profile metrics` Prometheus
- [x] PC-LT-06 `certs-init` one-shot: CA + api internal server cert + workload client cert (mTLS internal listener in production)
- [x] PC-LT-07 `init.sh` (.env with random secrets, admin password, master key; chmod 600; never regenerates) + `.env.example`
- [x] PC-LT-08 `README.md`: modes (production behind TLS / HTTP trial), choices, DR backup (pg dump + .env / master key)
- [x] PC-LT-09 `smoke.sh` (run later by the user; no live run in this change)
- [x] PC-LT-10 Static validation: YAML parse, service/env consistency against the binaries' config keys
- [x] PC-LT-13 `api health <url>` subcommand (Go HTTPS probe verified with the internal CA) used by the lite api healthcheck instead of busybox `wget` — `cmd/api/health_cmd.go`, `deploy/lite/docker-compose.yml`
- [x] PC-LT-11 Metrics off: backend/route-metrics reader degrade gracefully, UI does not error

## Docs
- [x] PC-LT-12 `docs/README.md` index + migration table note (3110)
