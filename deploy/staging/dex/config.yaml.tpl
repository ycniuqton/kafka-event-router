# Dex for staging, rendered by the dex image entrypoint (gomplate) from env set in docker-compose.staging.yml.
# The issuer is the public URL proxied by nginx (/dex), so browser and api resolve the same issuer.
issuer: {{ .Env.DEX_ISSUER }}
storage:
  type: memory
web:
  http: 0.0.0.0:5556
telemetry:
  http: 0.0.0.0:5558
oauth2:
  skipApprovalScreen: true
enablePasswordDB: true
staticClients:
  - id: event-router
    name: Event Router (staging)
    secretEnv: DEX_CLIENT_SECRET
    redirectURIs:
      - {{ .Env.DEX_REDIRECT_URI }}
staticPasswords:
  - email: {{ .Env.DEX_STAGING_USER_EMAIL }}
    hashFromEnv: DEX_STAGING_USER_HASH
    username: staging-admin
    userID: 5b0f3c1e-7f43-4d8e-9a55-1c2d3e4f5a6b
  - email: {{ .Env.DEX_STAGING_APPROVER_EMAIL }}
    hashFromEnv: DEX_STAGING_APPROVER_HASH
    username: staging-approver
    userID: 8c4e2a71-3d5b-4f0e-b6a9-2e7d1c9f0a3b
