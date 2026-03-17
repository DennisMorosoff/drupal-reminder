FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG COMMIT_HASH=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commitHash=${COMMIT_HASH}" \
    -o /out/sleepbot .

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S app && \
    adduser -S -G app app && \
    mkdir -p /data && \
    chown -R app:app /app /data

COPY --from=builder /out/sleepbot /app/sleepbot

ENV SLEEPBOT_DB_PATH=/data/sleepbot.db

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://127.0.0.1:8080/health || exit 1

USER app

ENTRYPOINT ["/app/sleepbot"]
