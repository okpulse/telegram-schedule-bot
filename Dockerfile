FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/bot ./cmd/bot

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /srv
COPY --from=build /bin/bot /usr/local/bin/bot
COPY ./.env ./.env
COPY ./data ./data
ENTRYPOINT ["/usr/local/bin/bot"]
