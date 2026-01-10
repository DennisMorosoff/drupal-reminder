---
name: go-optimization
description: Optimize Go code performance, manage concurrency, or configure Docker builds. Use when asking for "deployment", "dockerfile", or "make it faster".
---

# Go Optimization & Deployment

## 1. Docker Multi-Stage Build
Always use multi-stage builds to keep the image small (Alpine or Scratch).

```dockerfile
# Builder
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 is crucial for scratch/alpine compatibility
RUN CGO_ENABLED=0 GOOS=linux go build -o bot ./cmd/bot

# Runner
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/bot /bot
CMD ["/bot"]
```

## 2. Goroutines & Panics
Never start a goroutine without knowing how it will stop.

If a goroutine is critical, wrap it with a recovery function to prevent the entire bot from crashing on a panic.

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("Recovered from panic", "error", r)
        }
    }()
    // process task
}()
```

## 3. Memory Management
Avoid creating large structs inside tight loops.

Use sync.Pool if you are allocating and deallocating the same heavy objects frequently (e.g., buffers for image processing).
