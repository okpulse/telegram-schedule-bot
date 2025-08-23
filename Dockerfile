# ---- Build stage ----
FROM golang:1.23 AS build
WORKDIR /app

# Используем vendor: Railway не тянет сети
ENV GOFLAGS=-mod=vendor
ENV GOWORK=off
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Сначала мод‑файлы и vendor (лучше кешируется)
COPY go.mod go.sum ./
COPY vendor ./vendor

# Затем исходники
COPY . .

# Сборка статического бинаря + встроенные тайм‑зоны (без tzdata)
RUN go build -tags timetzdata -o /bin/bot ./cmd/bot

# ---- Runtime stage (distroless) ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /srv

COPY --from=build /bin/bot /usr/local/bin/bot

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]
