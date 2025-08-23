# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Сборка статического бинарника + встроенные часовые пояса
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless, без Alpine и apk) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

# Кладём бинарь
COPY --from=build /bin/bot /usr/local/bin/bot

# Запускаем под nonroot
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
