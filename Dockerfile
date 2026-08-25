# syntax=docker/dockerfile:1
# 888a2a Manager multi-stage container build: frontend + embedded machines -> manager
FROM node:24.12.0-slim AS frontend
WORKDIR /frontend-build
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN npm i -g pnpm@11.10.0 && pnpm i --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

FROM golang:1.26.5-alpine3.23 AS machine-build
ARG VERSION=local
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG A2A888_BUILD_PROXY
ENV VERSION=${VERSION} GIT_COMMIT=${GIT_COMMIT} BUILD_TIME=${BUILD_TIME}
ENV http_proxy=${A2A888_BUILD_PROXY} https_proxy=${A2A888_BUILD_PROXY} no_proxy=localhost,127.0.0.1
ENV HTTP_PROXY=${A2A888_BUILD_PROXY} HTTPS_PROXY=${A2A888_BUILD_PROXY} NO_PROXY=localhost,127.0.0.1
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN apk add --no-cache curl bash gzip unzip
RUN ./scripts/build-embedded-machines.sh /out/embedded_machine

FROM golang:1.26.5-alpine3.23 AS manager-build
ARG VERSION=local
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG A2A888_BUILD_PROXY
ARG RELEASE=false
ENV BUILD_TIME=${BUILD_TIME}
ENV http_proxy=${A2A888_BUILD_PROXY} https_proxy=${A2A888_BUILD_PROXY} no_proxy=localhost,127.0.0.1
ENV HTTP_PROXY=${A2A888_BUILD_PROXY} HTTPS_PROXY=${A2A888_BUILD_PROXY} NO_PROXY=localhost,127.0.0.1
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf ./backend/manager/server/dist ./backend/manager/server/embedded_machine
COPY --from=frontend /frontend-build/dist ./backend/manager/server/dist
COPY --from=machine-build /out/embedded_machine ./backend/manager/server/embedded_machine
RUN BUILD_TAGS="embed_frontend embed_machine" \
	&& if [ "${RELEASE}" = "true" ]; then BUILD_TAGS="${BUILD_TAGS} release"; fi \
	&& CGO_ENABLED=0 go build -tags "${BUILD_TAGS}" -ldflags "-w -s" -p=16 \
	-o /out/888a2a ./backend/manager/bin/server/main.go

FROM alpine:3.23 AS runner
ARG VERSION=local
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.revision=${GIT_COMMIT}
LABEL org.opencontainers.image.created=${BUILD_TIME}
RUN apk add --no-cache ca-certificates curl \
	&& adduser -D -u 1000 a2a888
COPY --from=manager-build /out/888a2a /usr/local/bin/888a2a
USER a2a888
EXPOSE 8181
ENV A2A888_PG_URL=
ENTRYPOINT ["888a2a"]
CMD ["--port", "8181"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
	CMD curl -fsS http://localhost:8181/healthz || exit 1
