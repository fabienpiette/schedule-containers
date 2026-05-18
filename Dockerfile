FROM golang:1.25 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /schedule-containers ./cmd/schedule-containers

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /schedule-containers /app/schedule-containers

VOLUME ["/data"]

EXPOSE 8080

ENTRYPOINT ["/app/schedule-containers"]
CMD ["serve"]