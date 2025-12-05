FROM node:20 AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.25 AS backend
WORKDIR /app

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/_site ./web/_site
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/dustin/Caddystat/internal/version.Version=${VERSION} \
    -X github.com/dustin/Caddystat/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/dustin/Caddystat/internal/version.BuildTime=${BUILD_TIME}" \
    -o /bin/caddystat ./cmd/caddystat

FROM gcr.io/distroless/base-debian12
COPY --from=backend /bin/caddystat /bin/caddystat
COPY --from=backend /app/web/_site /web/_site
EXPOSE 8404
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/bin/caddystat", "--healthcheck"]
ENTRYPOINT ["/bin/caddystat"]
