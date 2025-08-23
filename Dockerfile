# ---- Build stage ----
FROM golang:1.22 AS build
WORKDIR /app

# Сначала зависимости — кэш лучше работает
COPY go.mod go.sum ./
RUN go mod download

# Копируем остальной код
COPY . .

# Сборка статического бинарника + вшитая tzdata
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (no Alpine, no apk) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

# Кладём бинарь
COPY --from=build /bin/bot /usr/local/bin/bot

# Опционально: шаблон .env и начальную папку для БД
COPY ./.env ./.env
COPY ./data ./data

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
