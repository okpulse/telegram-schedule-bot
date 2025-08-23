# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Включаем vendor-режим для всех go-команд (не тянуть из сети)
ENV GOFLAGS=-mod=vendor
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Сначала мод-файлы и vendor (лучше кэшируется)
COPY go.mod go.sum ./
COPY vendor ./vendor

# Затем исходники
COPY . .

# Сборка статического бинаря + встроенные таймзоны (без tzdata пакета)
RUN go build -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

COPY --from=build /bin/bot /usr/local/bin/bot

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
