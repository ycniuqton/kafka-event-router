{{/* Chart name and fullname. */}}
{{- define "er.name" -}}{{ .Chart.Name }}{{- end -}}
{{- define "er.fullname" -}}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 50 | trimSuffix "-" }}{{- end -}}

{{/* Common labels; component labels add app.kubernetes.io/component. */}}
{{- define "er.labels" -}}
app.kubernetes.io/name: {{ include "er.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .root.Chart.Name .root.Chart.Version }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}
{{- define "er.selector" -}}
app.kubernetes.io/name: {{ include "er.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Image tag and image of a component: <registry>/<component>:<tag>. */}}
{{- define "er.tag" -}}{{ default .Chart.AppVersion .Values.image.tag }}{{- end -}}
{{- define "er.image" -}}
{{ printf "%s/%s:%s" .root.Values.image.registry .image (include "er.tag" .root) }}
{{- end -}}

{{/* Fails the render when a sensitive key is set as a plain value or no Secret is referenced. */}}
{{- define "er.validate" -}}
{{- if not .Values.secret.existingSecret }}
{{- fail "secret.existingSecret is required: create a Secret with the ROUTER_* credentials and reference it" }}
{{- end }}
{{- $plain := list .Values.env .Values.api.env .Values.worker.env .Values.preview.env }}
{{- range $key := .Values.secret.keys }}
{{- range $envMap := $plain }}
{{- if hasKey $envMap $key }}
{{- fail (printf "%s is sensitive: provide it through secret.existingSecret or secret.keyRefs, not env" $key) }}
{{- end }}
{{- end }}
{{- end }}
{{- if not (has .Values.worker.kind (list "StatefulSet" "Deployment")) }}
{{- fail "worker.kind must be StatefulSet or Deployment" }}
{{- end }}
{{- if and .Values.worker.cache.persistence.enabled (ne .Values.worker.kind "StatefulSet") }}
{{- fail "worker.cache.persistence.enabled requires worker.kind=StatefulSet" }}
{{- end }}
{{- end -}}

{{/* Env shared by Go components: config map + existing secret. */}}
{{- define "er.envFrom" -}}
- configMapRef:
    name: {{ include "er.fullname" . }}-env
- secretRef:
    name: {{ .Values.secret.existingSecret }}
{{- end -}}

{{/* Per-key Secret references (override the same key from existingSecret). */}}
{{- define "er.secretKeyRefs" -}}
{{- range $k, $ref := .Values.secret.keyRefs }}
- name: {{ $k }}
  valueFrom: { secretKeyRef: { name: {{ $ref.name }}, key: {{ $ref.key }} } }
{{- end }}
{{- end -}}

{{- define "er.componentEnv" -}}
{{- range $k, $v := . }}
- name: {{ $k }}
  value: {{ $v | quote }}
{{- end }}
{{- end -}}

{{/* TLS env for listeners (all Go components); clientCA only on the api. */}}
{{- define "er.tlsEnv" -}}
{{- if .root.Values.tls.enabled }}
- name: ROUTER_TLS_CERT_FILE
  value: {{ printf "%s/tls.crt" .root.Values.tls.mountPath }}
- name: ROUTER_TLS_KEY_FILE
  value: {{ printf "%s/tls.key" .root.Values.tls.mountPath }}
{{- if and .api .root.Values.tls.requireClientCerts }}
- name: ROUTER_TLS_CLIENT_CA_FILE
  value: {{ printf "%s/ca.crt" .root.Values.tls.mountPath }}
{{- end }}
{{- if not .api }}
- name: ROUTER_CONTROL_PLANE_CA_FILE
  value: {{ printf "%s/ca.crt" .root.Values.tls.mountPath }}
- name: ROUTER_CONTROL_PLANE_CLIENT_CERT_FILE
  value: {{ printf "%s/tls.crt" .root.Values.tls.mountPath }}
- name: ROUTER_CONTROL_PLANE_CLIENT_KEY_FILE
  value: {{ printf "%s/tls.key" .root.Values.tls.mountPath }}
{{- end }}
{{- end }}
{{- end -}}

{{- define "er.scheme" -}}{{ if .Values.tls.enabled }}https{{ else }}http{{ end }}{{- end -}}

{{/* Volumes every Go pod needs: writable /tmp and, with TLS, the certificate Secret. */}}
{{- define "er.goVolumeMounts" -}}
- { name: tmp, mountPath: /tmp }
{{- if .Values.tls.enabled }}
- { name: tls, mountPath: {{ .Values.tls.mountPath }}, readOnly: true }
{{- end }}
{{- end -}}
{{- define "er.goVolumes" -}}
- name: tmp
  emptyDir: {}
{{- if .Values.tls.enabled }}
- name: tls
  secret: { secretName: {{ .Values.tls.secretName }} }
{{- end }}
{{- end -}}

{{/* Projected ServiceAccount token volume for workload identity (worker, preview). */}}
{{- define "er.workloadTokenVolume" -}}
- name: workload-token
  projected:
    sources:
      - serviceAccountToken:
          audience: {{ .Values.workloadIdentity.audience }}
          expirationSeconds: {{ .Values.workloadIdentity.expirationSeconds }}
          path: token
{{- end -}}

{{/* /readyz, /healthz probes of the Go components (startup probe covers slow first boots). */}}
{{- define "er.probes" -}}
{{- $p := .root.Values.probes }}
{{- $scheme := upper (include "er.scheme" .root) }}
startupProbe:
  httpGet: { path: /healthz, port: http, scheme: {{ $scheme }} }
  periodSeconds: {{ $p.startup.periodSeconds }}
  failureThreshold: {{ $p.startup.failureThreshold }}
readinessProbe:
  httpGet: { path: /readyz, port: http, scheme: {{ $scheme }} }
  periodSeconds: {{ $p.readiness.periodSeconds }}
  failureThreshold: {{ $p.readiness.failureThreshold }}
livenessProbe:
  httpGet: { path: /healthz, port: http, scheme: {{ $scheme }} }
  periodSeconds: {{ $p.liveness.periodSeconds }}
  failureThreshold: {{ $p.liveness.failureThreshold }}
{{- end -}}

{{/* Pod placement: node selector, tolerations, anti-affinity and spread across nodes and zones. */}}
{{- define "er.scheduling" -}}
{{- $s := .root.Values.scheduling }}
{{- $v := index .root.Values .component }}
{{- $match := include "er.selector" . }}
{{- with merge (deepCopy (default dict $v.nodeSelector)) $s.nodeSelector }}
nodeSelector: {{- toYaml . | nindent 2 }}
{{- end }}
{{- with concat (default list $s.tolerations) (default list $v.tolerations) }}
tolerations: {{- toYaml . | nindent 2 }}
{{- end }}
{{- if $v.affinity }}
affinity: {{- toYaml $v.affinity | nindent 2 }}
{{- else if eq $s.antiAffinity "hard" }}
affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - topologyKey: kubernetes.io/hostname
        labelSelector:
          matchLabels: {{- $match | nindent 12 }}
{{- else if eq $s.antiAffinity "soft" }}
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          topologyKey: kubernetes.io/hostname
          labelSelector:
            matchLabels: {{- $match | nindent 14 }}
{{- end }}
topologySpreadConstraints:
  {{- range $key, $when := dict "kubernetes.io/hostname" $s.spread.hostname "topology.kubernetes.io/zone" $s.spread.zone }}
  - maxSkew: 1
    topologyKey: {{ $key }}
    whenUnsatisfiable: {{ $when }}
    labelSelector:
      matchLabels: {{- $match | nindent 8 }}
  {{- end }}
{{- end -}}

{{/* ServiceAccount annotations of a component (cloud workload identity for KMS etc.). */}}
{{- define "er.saAnnotations" -}}
{{- $v := index .root.Values .component }}
{{- with (and $v.serviceAccount $v.serviceAccount.annotations) }}
annotations: {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}
