GO        ?= go
BIN_DIR   ?= bin
CMDS      := api worker preview
API_ADDR     ?= :8080
WORKER_ADDR  ?= :8081
PREVIEW_ADDR ?= :8082
COMPOSE   := docker compose -f deploy/docker-compose.yml
IMAGE_TAG ?= local
OPENAPI   := api/openapi/openapi.yaml
HELM      ?= helm
CHART     := deploy/helm/event-router
KUBE_VERSION ?= 1.30.0

.PHONY: build test lint tidy run-api run-worker run-preview compose-up compose-down \
        up down migrate-up migrate-down migrate-status docker-build openapi-lint openapi-gen clean test-sandbox \
        smoke test-integration helm-lint helm-template

build:
	@mkdir -p $(BIN_DIR)
	@for c in $(CMDS); do $(GO) build -o $(BIN_DIR)/$$c ./cmd/$$c || exit 1; done

test:
	$(GO) test ./...

# Release gate of the JS sandbox: escape, golden, determinism, cross-route and timeout suites.
test-sandbox:
	$(GO) test ./pkg/sandbox/ ./pkg/sandbox/escapetest/ -count=1 -run "Golden|Determinism|Escape|CrossRoute|Timeout|LibrarySourced|Replay|HotReload"

lint:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

run-api:
	ROUTER_HTTP_ADDR=$(API_ADDR) $(GO) run ./cmd/api

run-worker:
	ROUTER_HTTP_ADDR=$(WORKER_ADDR) $(GO) run ./cmd/worker

run-preview:
	ROUTER_HTTP_ADDR=$(PREVIEW_ADDR) $(GO) run ./cmd/preview

# Infrastructure only (Postgres, Redis, Kafka, Prometheus) for running binaries on the host.
compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

# Full local stack: infra + migrate job + api/worker/preview/web (profile "app").
up:
	$(COMPOSE) --profile app up -d --build

down:
	$(COMPOSE) --profile app --profile oidc down

# Waits until every service of `make up` answers /readyz 200 (CI compose-smoke job).
smoke:
	deploy/docker/smoke.sh

# Integration tests on the testkit harness: ROUTER_TEST_PROVIDER=container (docker) or ROUTER_TEST_DATABASE_URL.
test-integration:
	$(GO) test -count=1 ./internal/testkit/ ./internal/store/... ./internal/audit/...

migrate-up:
	$(GO) run ./cmd/api migrate up

migrate-down:
	$(GO) run ./cmd/api migrate down 1

migrate-status:
	$(GO) run ./cmd/api migrate status

docker-build:
	@for c in $(CMDS); do \
		docker build -f deploy/docker/go.Dockerfile --build-arg CMD=$$c -t event-router/$$c:$(IMAGE_TAG) . || exit 1; \
	done
	docker build -f deploy/docker/web.Dockerfile -t event-router/web:$(IMAGE_TAG) .

openapi-lint:
	npx --yes @redocly/cli@1 lint $(OPENAPI) --config api/openapi/redocly.yaml

openapi-gen:
	cd web && npm run api:gen

clean:
	rm -rf $(BIN_DIR)

# Pipeline engine code generation: Go + TS route config types and the static engine catalogue.
SCHEMA_DIR    := pkg/config/schema
GEN_WEB_DIR   := web/src/api/generated
GEN_SCRATCH   ?= .local/check-gen
GEN_BANNER    := /* generated from pkg/config/schema by make gen, do not edit */
JSON2TS       := npx --yes json-schema-to-typescript@15.0.4 --unreachableDefinitions --no-additionalProperties --maxItems -1
GOJSONSCHEMA  := go-jsonschema -p config --only-models -t --tags json --capitalization API,ID,JS,JSON,DLQ

.PHONY: gen gen-types gen-ts gen-ts-into gen-catalog check-gen engine-bench-gate engine-gate

gen: gen-types gen-ts gen-catalog

gen-types:
	$(GOJSONSCHEMA) -o pkg/config/routeconfig_gen.go $(SCHEMA_DIR)/route.v1.schema.json

gen-ts:
	$(MAKE) --no-print-directory gen-ts-into OUT=$(GEN_WEB_DIR)

gen-ts-into:
	@mkdir -p $(OUT)
	$(GO) run ./cmd/gen-tsschema $(SCHEMA_DIR)/route.v1.schema.json | $(JSON2TS) --bannerComment '$(GEN_BANNER)' > $(OUT)/routeConfig.ts
	$(GO) run ./cmd/gen-tsschema $(SCHEMA_DIR)/engine-types.v1.schema.json | $(JSON2TS) --bannerComment '$(GEN_BANNER)' > $(OUT)/engineTypes.ts

gen-catalog:
	$(GO) run ./cmd/gen-catalog -out $(GEN_WEB_DIR)/engineCatalog.json -ts-out $(GEN_WEB_DIR)/engineCatalog.ts

check-gen:
	@rm -rf $(GEN_SCRATCH) && mkdir -p $(GEN_SCRATCH)
	$(GOJSONSCHEMA) -o $(GEN_SCRATCH)/routeconfig_gen.go $(SCHEMA_DIR)/route.v1.schema.json
	$(MAKE) --no-print-directory gen-ts-into OUT=$(GEN_SCRATCH)
	$(GO) run ./cmd/gen-catalog -out $(GEN_SCRATCH)/engineCatalog.json -ts-out $(GEN_SCRATCH)/engineCatalog.ts
	diff -u pkg/config/routeconfig_gen.go $(GEN_SCRATCH)/routeconfig_gen.go
	@for f in routeConfig.ts engineTypes.ts engineCatalog.json engineCatalog.ts; do diff -u $(GEN_WEB_DIR)/$$f $(GEN_SCRATCH)/$$f || exit 1; done

engine-bench-gate:
	ROUTER_BENCH_GATE=1 $(GO) test ./pkg/engine/ -run TestBenchThresholds -count=1

# Engine release gate (PE-112): unit + golden tests, every engine fuzz target for ENGINE_FUZZTIME, bench thresholds.
ENGINE_FUZZTIME     ?= 60s
ENGINE_FUZZ_TARGETS := ./pkg/engine/codec:FuzzJSONCodec ./pkg/engine/codec:FuzzAvroDecode ./pkg/engine/codec:FuzzProtobufDecode \
	./pkg/engine/path:FuzzFieldPath ./pkg/engine/filter:FuzzFilterASTRoundTrip ./pkg/engine/formula:FuzzFormulaParser \
	./pkg/engine/step/recipes:FuzzRecipesNoPanic

engine-gate:
	$(GO) test -count=1 ./pkg/config/... ./pkg/engine/... ./cmd/routerctl/
	@for t in $(ENGINE_FUZZ_TARGETS); do pkg=$${t%%:*}; fn=$${t##*:}; \
		echo "fuzz $$fn in $$pkg for $(ENGINE_FUZZTIME)"; \
		$(GO) test $$pkg -run '^$$' -fuzz "^$$fn\$$" -fuzztime $(ENGINE_FUZZTIME) || exit 1; done
	$(MAKE) --no-print-directory engine-bench-gate

# Helm chart: lint both value sets, then validate rendered manifests against the Kubernetes schemas.
helm-lint:
	$(HELM) lint $(CHART)
	$(HELM) lint $(CHART) -f $(CHART)/ci/tls-values.yaml
	$(HELM) lint $(CHART) -f $(CHART)/ci/ha-values.yaml

helm-template:
	$(HELM) template event-router $(CHART) | kubeconform -strict -summary -kubernetes-version $(KUBE_VERSION) -
	$(HELM) template event-router $(CHART) -f $(CHART)/ci/tls-values.yaml | kubeconform -strict -summary -kubernetes-version $(KUBE_VERSION) -
	$(HELM) template event-router $(CHART) -f $(CHART)/ci/ha-values.yaml | kubeconform -strict -summary -kubernetes-version $(KUBE_VERSION) -
