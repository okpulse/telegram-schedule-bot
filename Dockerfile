# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Используем vendor-режим, чтобы не тянуть сеть
ENV GOFLAGS=-mod=vendor
ENV GOWORK=off
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# 1) Сначала моды и vendor (лучшая кешируемость)
COPY go.mod go.sum ./
COPY vendor ./vendor

# 2) Копируем исходники
COPY . .

# 3) Диагностика окружения и содержимого (это пойдёт в логи Railway)
RUN echo "== go env ==" && go env
RUN echo "== tree (top) ==" && ls -la
RUN echo "== tree cmd/bot ==" && ls -la cmd/bot || true
RUN echo "== tree internal ==" && ls -la internal || true
RUN test -f vendor/modules.txt && echo "vendor/modules.txt OK" || (echo "NO vendor/modules.txt" && exit 1)
RUN echo "== go list deps (cmd/bot) ==" && go list -deps ./cmd/bot || true
RUN echo "== go mod vendor sanity ==" && grep -E '^# ' vendor/modules.txt | head -n 20 || true

# 4) Сборка с максимально подробным выводом
#    (-v: verbose, -x: печать команд, -tags timetzdata: встроенные тайм-зоны)
RUN echo "== go build verbose ==" && go build -v -x -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

COPY --from=build /bin/bot /usr/local/bin/bot

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
