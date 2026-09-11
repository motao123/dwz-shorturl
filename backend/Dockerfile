# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the server and the migration tool. The migration binary is shipped so a
# deployment can migrate a database without a Go toolchain (and without the
# repo checked out next to it).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app/server ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app/migrate ./cmd/migrate

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app

# Copy binary, configs and the migration set. The migrations are copied so a
# running container can be upgraded in place with the bundled tool:
#   ./migrate -status            # list applied / pending migrations
#   ./migrate -all               # apply every pending migration
# The directory is read relative to WORKDIR, matching the -migrations default.
COPY --from=builder /app/server .
COPY --from=builder /app/migrate .
COPY configs/ ./configs/
COPY migrations/ ./migrations/

# Drop privileges: run as an unprivileged user so a container escape is not root.
USER app

EXPOSE 8080

ENTRYPOINT ["./server"]
