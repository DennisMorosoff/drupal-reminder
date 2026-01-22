#!/bin/bash

# Скрипт сборки Drupal Reminder Bot с автоматическим определением версии из git

set -e

# Определение версии из git тегов
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Определение времени сборки в формате ISO 8601
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Определение короткого хеша коммита
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building Drupal Reminder Bot..."
echo "Version: $VERSION"
echo "Build time: $BUILD_TIME"
echo "Commit: $COMMIT"
echo ""

# Сборка с установкой версии через ldflags
go build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.commitHash=$COMMIT" -o drupal-reminder-bot main.go

echo ""
echo "✅ Build completed successfully!"
echo "Binary: drupal-reminder-bot"
