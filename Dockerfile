# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Используем vendor (чтобы Railway не тянул сети)
ENV GOFLAGS=-mod=vendor
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# 1) Кэшируем мод-файлы и vendor для повторных сборок
COPY go.mod go.sum ./
COPY vendor ./vendor

# 2) Копируем исходники
COPY . .

# 3) Диагностика окружения (это попадёт в логи Railway)
RUN go env
RUN ls -la
RUN ls -la cmd/bot || true
RUN ls -la internal || true
RUN test -f vendor/modules.txt && echo "vendor/modules.txt OK" || (echo "NO vendor/modules.txt" && exit 1)

# 4) Диагностическая сборка с подробным выводом
RUN go build -v -x -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv
COPY --from=build /bin/bot /usr/local/bin/bot
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
