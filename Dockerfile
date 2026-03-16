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

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && \
    adduser -S -G app app && \
    mkdir -p /data && \
    chown -R app:app /app /data

COPY --from=builder /out/sleepbot /app/sleepbot

ENV SLEEPBOT_DB_PATH=/data/sleepbot.db

VOLUME ["/data"]

USER app

ENTRYPOINT ["/app/sleepbot"]
