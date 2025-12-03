FROM node:20 AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.25 AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/_site ./web/_site
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/caddystat ./cmd/caddystat

FROM gcr.io/distroless/base-debian12
COPY --from=backend /bin/caddystat /bin/caddystat
COPY --from=backend /app/web/_site /web/_site
EXPOSE 8000
ENTRYPOINT ["/bin/caddystat"]
