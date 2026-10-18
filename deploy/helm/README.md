# Helm chart `event-router`

`deploy/helm/event-router/` installs:

| Component | Kind | Notes |
|---|---|---|
| api | Deployment (2 replicas, HPA 2-6, PDB) | Services `-api` (public) and `-api-internal` (workers/preview only); TokenReview binding for `k8s_token` |
| worker | StatefulSet (2 replicas, PDB `maxUnavailable: 1`) | `ROUTER_WORKER_ID` = pod name (downward API), `ROUTER_WORKER_ZONE` from the pod's `topology.kubernetes.io/zone` label, cache volume (emptyDir or per-pod PVC), headless Service |
| preview | Deployment | projected token audience `event-router-internal` |
| web | Deployment (nginx) | chart-owned nginx config proxying `/api`, `/auth` to the api Service |
| migrate | pre-install/pre-upgrade hook Job | `api migrate up` |

Plus ServiceAccounts, an optional Ingress (`/api`, `/auth` → api; `/` → web), default-deny NetworkPolicies and
pod anti-affinity + topology spread across nodes and zones (`scheduling.*`).

Backing services (Postgres, Redis, Kafka, Vault/KMS, IdP) are not bundled: plain endpoints go to `external.*`,
credentials only to a pre-existing Secret (`secret.existingSecret`, keys = `ROUTER_*` names) or per-key
`secret.keyRefs`. The chart never renders secret values and fails the render if a sensitive key appears in `env`.

```sh
kubectl -n event-router create secret generic event-router-secrets \
  --from-literal=ROUTER_DATABASE_URL='postgres://...' --from-literal=ROUTER_REDIS_URL='redis://...' \
  --from-literal=ROUTER_COOKIE_SECRET=... --from-literal=ROUTER_CURSOR_SECRET=... \
  --from-literal=ROUTER_PREVIEW_DIRECTIVE_SECRET=... --from-literal=ROUTER_OIDC_CLIENT_SECRET=... \
  --from-literal=ROUTER_VAULT_TOKEN=...
helm install er deploy/helm/event-router -n event-router \
  --set global.publicURL=https://router.example.com --set ingress.host=router.example.com \
  --set external.vault.addr=https://vault:8200 --set external.oidc.issuer=https://idp.example.com
```

Value sets: defaults; `ci/ha-values.yaml` (3 api / 3 workers, hard anti-affinity, `DoNotSchedule` zone spread,
persistent worker cache, TLS + mTLS, DB credentials from an operator Secret); `ci/tls-values.yaml`;
`ci/kind-values.yaml` (kind smoke, Secret from `ci/kind-deps.yaml`).

```sh
helm lint deploy/helm/event-router -f deploy/helm/event-router/ci/ha-values.yaml      # make helm-lint
helm template er deploy/helm/event-router | kubeconform -strict -summary -             # make helm-template
```

Images: `<image.registry>/{api,worker,preview,web}:<tag>` built from `deploy/docker/{go,web}.Dockerfile`.
Worker zone: Kubernetes copies the node's zone label onto pods with the PodTopologyLabelsAdmission feature
(1.33+); without it the variable is empty and the worker reports zone `default` — set
`worker.zone.fromTopologyLabel=false` and `worker.zone.value` to pin a zone instead.
TLS/mTLS: `tls.enabled=true` mounts `tls.secretName` (`tls.crt`, `tls.key`, `ca.crt`) into every Go pod (web
gets `ca.crt` to verify the api); the api's internal listener then requires client certificates and
`env.ROUTER_WORKLOAD_AUTH=mtls` can replace `k8s_token`. CI: `.github/workflows/helm.yml`.
