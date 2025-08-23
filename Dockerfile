# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Включаем vendor-режим для всех go-команд
ENV GOFLAGS=-mod=vendor
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Копируем мод-файлы и vendor заранее (лучше кэшируется)
COPY go.mod go.sum ./
COPY vendor ./vendor

# Копируем исходники
COPY . .

# Сборка статического бинарника + встроенные таймзоны
RUN go build -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

COPY --from=build /bin/bot /usr/local/bin/bot

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
