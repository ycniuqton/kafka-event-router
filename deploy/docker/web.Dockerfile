# syntax=docker/dockerfile:1.7
# Builds the React SPA and serves it with nginx (SPA fallback, /api and /auth proxied to the api).
FROM node:22-alpine AS build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
COPY testdata/rbac /testdata/rbac
COPY testdata/approvals /testdata/approvals
COPY internal/preview/testdata/patch /internal/preview/testdata/patch
ARG VITE_DATA_SOURCE=http
ENV VITE_DATA_SOURCE=${VITE_DATA_SOURCE}
RUN npm run build

FROM nginx:1.27-alpine
COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
EXPOSE 8080
