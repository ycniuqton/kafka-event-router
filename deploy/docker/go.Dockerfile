# syntax=docker/dockerfile:1.7
# One image recipe for every Go binary; ARG CMD selects cmd/<CMD> (api, worker, preview).
# RUNTIME_IMAGE may be swapped for distroless :debug-nonroot where container healthchecks need busybox wget.
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
ARG CMD=api
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}
RUN mkdir -p /out/state/cache

FROM ${RUNTIME_IMAGE}
COPY --from=build /out/app /app
COPY --from=build --chown=nonroot:nonroot /out/state /var/lib/router
USER nonroot:nonroot
ENTRYPOINT ["/app"]
