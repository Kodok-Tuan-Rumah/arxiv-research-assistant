FROM golang:1.25.14-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /bin/server ./cmd/server/main.go

# Run stage
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/server /bin/server
EXPOSE 8080
ENV DB_HOST=postgres \
  DB_PORT=5432 \
  DB_USER=arxiv \
  DB_PASSWORD=arxiv_secret \
  DB_NAME=arxiv_db \
  ML_SERVICE_URL=http://python-ml:8001 \
  QDRANT_URL=http://qdrant:6333

# wget localhost resolves to ::1 while Fiber binds 0.0.0.0 (IPv4) — probe the IPv4 loopback explicitly.
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1
CMD ["/bin/server"]