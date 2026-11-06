# syntax=docker/dockerfile:1

# ---- web UI (Node is needed only at build time) ----
FROM node:26-alpine AS web
RUN npm install -g pnpm@12.16.1
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- Go binary (static, no CGO) ----
FROM golang:1.27-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
	-ldflags "-s -w -X github.com/thomasbudiarjo/envily/internal/version.Version=${VERSION} -X github.com/thomasbudiarjo/envily/internal/version.Commit=${COMMIT} -X github.com/thomasbudiarjo/envily/internal/version.Date=${DATE}" \
	-o /out/envily ./cmd/envily \
	&& mkdir -p /out/data

# ---- runtime: one static binary, one volume ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/envily /usr/local/bin/envily
COPY --from=go --chown=65532:65532 /out/data /data
ENV ENVILY_ADDR=:8080 \
	ENVILY_DB_DSN=/data/envily.db
VOLUME ["/data"]
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD ["/usr/local/bin/envily", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/envily"]
CMD ["server"]
